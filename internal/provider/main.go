package provider

import (
	"io"
	"log/slog"
	"regexp"

	"sigs.k8s.io/external-dns/endpoint"
)

// Options to provide to the main entry point [New]
type Opts struct {
	FilterExclude      []string
	FilterInclude      []string
	FilterRegexExclude *regexp.Regexp
	FilterRegexInclude *regexp.Regexp
	Logger             *slog.Logger
	RouterOSAddress    string
	RouterOSPassword   string
	RouterOSUsername   string
	ServerHost         string
	ServerPort         uint
}

// Initializes the application and returns the configured [server] exposing the provider webhook.
func New(o *Opts) (*server, error) {
	l := o.Logger
	if l == nil {
		l = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	pc, err := NewClient(&ClientOpts{
		Address:  o.RouterOSAddress,
		Logger:   l.With("name", "client"),
		Password: o.RouterOSPassword,
		Username: o.RouterOSUsername,
	})
	if err != nil {
		return nil, err
	}

	var df endpoint.DomainFilter
	if o.FilterRegexExclude != nil || o.FilterRegexInclude != nil {
		df = endpoint.NewRegexDomainFilter(o.FilterRegexInclude, o.FilterRegexExclude)
	} else if o.FilterExclude != nil {
		df = endpoint.NewDomainFilterWithExclusions(o.FilterInclude, o.FilterExclude)
	} else {
		df = endpoint.NewDomainFilter(o.FilterInclude)
	}
	p, err := NewProvider(&ProviderOpts{
		Client:       pc,
		DomainFilter: df,
		Logger:       l.With("name", "provider"),
	})
	if err != nil {
		return nil, err
	}

	s, err := NewServer(&ServerOpts{
		Host:     o.ServerHost,
		Logger:   l.With("name", "server"),
		Port:     o.ServerPort,
		Provider: p,
	})
	if err != nil {
		return nil, err
	}

	return s, nil
}
