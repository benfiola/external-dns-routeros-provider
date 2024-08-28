package provider

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/go-routeros/routeros/v3"
	"sigs.k8s.io/external-dns/endpoint"
)

// The public interface for the routeros client
type Client interface {
	Health() error
	ListEndpoints() ([]*endpoint.Endpoint, error)
	CreateEndpoint(e *endpoint.Endpoint) error
	DeleteEndpoint(e *endpoint.Endpoint) error
}

// The internal struct for a routeros client holding state and configuration.
type client struct {
	address  string
	client   *routeros.Client
	logger   *slog.Logger
	password string
	username string
}

// Options passed to [NewClient] when creating a new [client].
type ClientOpts struct {
	Address  string
	Logger   *slog.Logger
	Password string
	Username string
}

// Creates a new [client] struct using the provided [ClientOpts] arguments.
// Validates that the provided options are valid.
func NewClient(o ClientOpts) (client, error) {
	l := o.Logger
	if l == nil {
		l = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	cs := strings.Split(o.Address, ":")
	if len(cs) != 2 {
		return client{}, fmt.Errorf("address not <host>:<port> format")
	}
	_, err := strconv.ParseUint(cs[1], 0, 0)
	if err != nil {
		return client{}, fmt.Errorf("port invalid: %w", err)
	}
	return client{
		address:  o.Address,
		logger:   l,
		password: o.Password,
		username: o.Username,
	}, nil
}

// Callback used as part of the [withClient] implementation
type withClientCallback func() error

// Function that handles opening and closing a connection to routeros.
// Attaches the connected client to the parent [client] object.
func (c *client) withClient(cb withClientCallback) error {
	cc := c.client == nil
	if cc {
		rc, err := routeros.Dial(c.address, c.username, c.password)
		if err != nil {
			return err
		}
		c.client = rc
	}

	defer func() {
		if cc {
			cl := c.client
			c.client = nil
			cl.Close()
		}
	}()

	return cb()
}

// Performs a health check of the client by querying a simple routeros api
// If the query fails and returns an error, this indicates the client is unhealthy
func (c client) Health() error {
	return c.withClient(func() error {
		_, err := c.client.RunArgs([]string{"/system/resource/print"})
		return err
	})
}

// Internal method that calls routeros '/ip/dns/static/add' with a [map[string]string] that should have the same shape as a routeros ip dns record.
// Returns an error if the api call fails
func (c *client) createDnsRecord(v map[string]string) error {
	return c.withClient(func() error {
		c.logger.Debug(fmt.Sprintf("create routeros dns record %s %s", v["type"], v["name"]))
		cmd := []string{"/ip/dns/static/add"}
		for k, v := range v {
			attr := fmt.Sprintf("=%s=%s", k, v)
			cmd = append(cmd, attr)
		}
		_, err := c.client.RunArgs(cmd)
		return err
	})
}

// Internal method that calls routeros '/ip/dns/static/remove' with a [map[string]string] that should have the same shape as a routeros ip dns record.
// Returns an error if the api call fails
func (c *client) deleteDnsRecord(v map[string]string) error {
	return c.withClient(func() error {
		c.logger.Debug(fmt.Sprintf("delete routeros dns record %s", v[".id"]))
		cmd := []string{"/ip/dns/static/remove"}
		cmd = append(cmd, fmt.Sprintf("=.id=%s", v[".id"]))
		_, err := c.client.RunArgs(cmd)
		return err
	})
}

// Metadata stored as a comment within a routeros dns record
type recordMetadata struct{}

// When a routeros dns record is missing metadata via structured data stored in its comment,
// it's because either a) the record is not managed by external-dns or b) the record is invalid.
// If the record is invalid, the provider should attempt to clean it up.
// If the record is not managed by external-dns, it should be ignored.
// This is a specialized error type that helps disambiguate between these two cases.
type NotExternalDnsRecordError struct {
	Id string
}

func (e NotExternalDnsRecordError) Error() string {
	return fmt.Sprintf("dns record %s not managed by external-dns", e.Id)
}

// If a routeros dns record comment starts with this prefix, its managed by the provider.
var recordMetadataPrefix = "external-dns:"

// Retrieves metadata from a routeros dns record
func (c client) getRecordMetadata(v map[string]string) (recordMetadata, error) {
	co := v["comment"]
	rms, ok := strings.CutPrefix(co, recordMetadataPrefix)
	if !ok {
		// comment does not start with prefix - is not managed by external-dns
		return recordMetadata{}, NotExternalDnsRecordError{Id: v[".id"]}
	}
	rm := recordMetadata{}
	err := json.Unmarshal([]byte(rms), &rm)
	if err != nil {
		// record is managed by external-dns, but metadata is not parseable
		return recordMetadata{}, err
	}
	return rm, nil
}

// Internal method that calls routeros '/ip/dns/static/print' api.
// Filters out records that aren't managed by external-dns.
// Adds default data to records fetched from routeros.
// Returns an error if the api call fails.
// Returns an error if cleaning up invalid records fail.
func (c *client) listDnsRecords() ([]map[string]string, error) {
	c.logger.Debug("list routeros dns records")
	rs := []map[string]string{}
	irs := []map[string]string{}
	err := c.withClient(func() error {
		rep, err := c.client.RunArgs([]string{"/ip/dns/static/print", "=detail"})
		if err != nil {
			return err
		}
		for _, s := range rep.Re {
			r := s.Map
			_, err := c.getRecordMetadata(r)
			if err != nil {
				_, nedre := err.(NotExternalDnsRecordError)
				if nedre {
					c.logger.Debug(fmt.Sprintf("ignore non-external dns record %s", r[".id"]))
					continue
				}
				c.logger.Debug(fmt.Sprintf("delete malformed dns record %s", r[".id"]))
				c.deleteDnsRecord(r)
				continue
			}
			// A records are the default record type
			if r["type"] == "" {
				r["type"] = "A"
			}
			rs = append(rs, r)
		}
		return nil
	})
	if err != nil {
		return []map[string]string{}, err
	}

	for _, r := range irs {
		err := c.deleteDnsRecord(r)
		if err != nil {
			return []map[string]string{}, err
		}
	}

	return rs, nil
}

// A key is used to connect [endpoint.Endpoint] and routeros ip dns records.
// This function standardizes on this key.
func (c client) makeKey(rt string, n string) string {
	return fmt.Sprintf("%s::%s", rt, n)
}

// Creates a new endpoint
func (c client) CreateEndpoint(e *endpoint.Endpoint) error {
	rm := recordMetadata{}
	rmb, err := json.Marshal(rm)
	if err != nil {
		// external-dns managed record is invalid - unexpected, should be caught by [listDnsRecords]
		return err
	}
	com := fmt.Sprintf("%s%s", recordMetadataPrefix, string(rmb))
	for _, t := range e.Targets {
		r := map[string]string{
			"comment": com,
			"name":    e.DNSName,
			"type":    e.RecordType,
			"ttl":     time.Duration(e.RecordTTL * 1e9).String(),
		}
		switch e.RecordType {
		case "A":
			r["address"] = t
		case "CNAME":
			r["cname"] = t
		case "MX":
			ps := strings.Split(t, " ")
			if len(ps) != 2 {
				return fmt.Errorf("malformed mx record %s", t)
			}
			r["mx-preference"] = ps[0]
			r["mx-exchange"] = ps[1]
		case "NS":
			r["ns"] = t
		case "SRV":
			ps := strings.Split(t, " ")
			if len(ps) != 4 {
				return fmt.Errorf("malformed srv record %s", t)
			}
			r["srv-priority"] = ps[0]
			r["srv-weight"] = ps[1]
			r["srv-port"] = ps[2]
			r["srv-target"] = ps[3]
		case "TXT":
			r["text"] = t
		default:
			return fmt.Errorf("unsupported record type %s", e.RecordType)
		}
		err = c.createDnsRecord(r)
		if err != nil {
			return err
		}
	}
	return nil
}

// Deletes an endpoint
func (c client) DeleteEndpoint(e *endpoint.Endpoint) error {
	rs, err := c.listDnsRecords()
	if err != nil {
		return err
	}
	k := c.makeKey(e.RecordType, e.DNSName)
	for _, r := range rs {
		_, err := c.getRecordMetadata(r)
		if err != nil {
			// external-dns managed record is invalid - unexpected, should be caught by [listDnsRecords]
			return err
		}
		rk := c.makeKey(r["type"], r["name"])
		if rk != k {
			// record is not mapped to endpoint - ignore
			continue
		}
		err = c.deleteDnsRecord(r)
		if err != nil {
			return err
		}
	}
	return nil
}

// Lists all endpoints
func (c client) ListEndpoints() ([]*endpoint.Endpoint, error) {
	rs, err := c.listDnsRecords()
	if err != nil {
		return []*endpoint.Endpoint{}, err
	}
	mes := map[string]*endpoint.Endpoint{}
	for _, r := range rs {
		_, err := c.getRecordMetadata(r)
		if err != nil {
			// external-dns managed record is invalid - unexpected, should be caught by [listDnsRecords]
			return []*endpoint.Endpoint{}, err
		}
		k := c.makeKey(r["type"], r["name"])
		_, ex := mes[k]
		if !ex {
			ttl, err := time.ParseDuration(r["ttl"])
			if err != nil {
				return []*endpoint.Endpoint{}, err
			}
			mes[k] = &endpoint.Endpoint{
				DNSName:    r["name"],
				RecordTTL:  endpoint.TTL(ttl),
				RecordType: r["type"],
				Targets:    []string{},
			}
		}
		e := mes[k]
		switch r["type"] {
		case "A":
			e.Targets = append(e.Targets, r["address"])
		case "CNAME":
			e.Targets = append(e.Targets, r["cname"])
		case "MX":
			t := fmt.Sprintf("%s %s", r["mx-preference"], r["mx-exchange"])
			e.Targets = append(e.Targets, t)
		case "NS":
			e.Targets = append(e.Targets, r["ns"])
		case "SRV":
			t := fmt.Sprintf("%s %s %s %s", r["srv-priority"], r["srv-weight"], r["srv-port"], r["srv-target"])
			e.Targets = append(e.Targets, t)
		case "TXT":
			e.Targets = append(e.Targets, r["text"])
		default:
			return []*endpoint.Endpoint{}, fmt.Errorf("unsupported record type %s", r["type"])
		}
	}
	es := []*endpoint.Endpoint{}
	for _, v := range mes {
		es = append(es, v)
	}
	return es, nil
}
