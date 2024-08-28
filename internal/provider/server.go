package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/labstack/echo/v4"
	slogecho "github.com/samber/slog-echo"
	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
)

// Internal server data struct that binds a [Provider] to endpoint functions
type server struct {
	echo     *echo.Echo
	host     string
	logger   *slog.Logger
	port     uint
	provider Provider
}

// Function that handles parsing a request into the given [interface{}] object
type requestReader func(data interface{}) error

// Uses a request's 'Content-Type' header to produce a matching `[requestReader]` function.
// Raises an [error] if the 'Content-Type' header is unrecognized.
func (s *server) getRequestReader(c echo.Context) (requestReader, error) {
	value := c.Request().Header.Get("Content-Type")
	switch value {
	case "application/json":
		return func(data interface{}) error {
			return json.NewDecoder(c.Request().Body).Decode(&data)
		}, nil
	case "application/external.dns.webhook+json;version=1":
		return func(data interface{}) error {
			return json.NewDecoder(c.Request().Body).Decode(&data)
		}, nil
	default:
		return nil, fmt.Errorf("unrecognized content-type header %s", value)
	}
}

// Function that handles creating a response with the given data and HTTP status code
type responseWriter func(code int, data interface{}) error

// Uses a request's 'Accept' header to produce a matching [responseWriter] function.
// Raises an [error] if the 'Accept' header is unrecognized.
func (s *server) getResponseWriter(c echo.Context) (responseWriter, error) {
	value := c.Request().Header.Get("Accept")
	switch value {
	case "application/json":
		return c.JSON, nil
	case "application/external.dns.webhook+json;version=1":
		return func(code int, data interface{}) error {
			c.Response().Header().Set(echo.HeaderContentType, "application/external.dns.webhook+json;version=1")
			c.Response().WriteHeader(http.StatusOK)
			bs, err := json.Marshal(data)
			if err != nil {
				return err
			}
			c.Response().Write(bs)
			return nil
		}, nil
	default:
		return nil, fmt.Errorf("unrecognized accept header %s", value)
	}
}

// Webhook endpoint function calling [Provider.AdjustEndpoints]
func (s *server) adjustEndpoints(c echo.Context) error {
	rr, err := s.getRequestReader(c)
	if err != nil {
		return err
	}
	rw, err := s.getResponseWriter(c)
	if err != nil {
		return err
	}
	body := []*endpoint.Endpoint{}
	err = rr(&body)
	if err != nil {
		return err
	}
	es, err := s.provider.AdjustEndpoints(body)
	if err != nil {
		return err
	}
	return rw(http.StatusOK, es)
}

// Webhook endpoint function calling [Provider.ApplyChanges]
func (s *server) applyChanges(c echo.Context) error {
	rr, err := s.getRequestReader(c)
	if err != nil {
		return err
	}
	body := plan.Changes{}
	err = rr(&body)
	if err != nil {
		return err
	}
	err = s.provider.ApplyChanges(context.Background(), &body)
	if err != nil {
		return err
	}
	return c.NoContent(http.StatusNoContent)
}

// Webhook endpoint function calling [Provider.Health]
func (s *server) health(c echo.Context) error {
	err := s.provider.Health()
	if err != nil {
		return err
	}
	return c.NoContent(http.StatusOK)
}

// Webhook endpoint function calling [Provider.Records]
func (s *server) records(c echo.Context) error {
	rw, err := s.getResponseWriter(c)
	if err != nil {
		return err
	}
	rs, err := s.provider.Records(context.Background())
	if err != nil {
		return err
	}
	return rw(http.StatusOK, rs)
}

// Webhook endpoint function calling [Provider.GetDomainFilter]
func (s *server) getDomainFilter(c echo.Context) error {
	rw, err := s.getResponseWriter(c)
	if err != nil {
		return err
	}
	df := s.provider.GetDomainFilter()
	return rw(http.StatusOK, df)
}

// Options provided to [NewServer]
type ServerOpts struct {
	Host     string
	Logger   *slog.Logger
	Port     uint
	Provider Provider
}

// Constructs an [server] using the provided options within [ServerOpts]
func NewServer(o *ServerOpts) (*server, error) {
	l := o.Logger
	if l == nil {
		l = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if o.Provider == nil {
		return nil, fmt.Errorf("")
	}
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	s := server{
		echo:     e,
		host:     o.Host,
		logger:   l,
		port:     o.Port,
		provider: o.Provider,
	}
	e.Use(slogecho.New(l))
	e.GET("/", s.getDomainFilter)
	e.POST("/adjustendpoints", s.adjustEndpoints)
	e.GET("/healthz", s.health)
	e.GET("/records", s.records)
	e.POST("/records", s.applyChanges)
	return &s, nil
}

// Runs the [server] using its internal configuration
func (s *server) Run() error {
	h := s.host
	if h == "" {
		h = "127.0.0.1"
	}
	p := s.port
	if p == 0 {
		p = 8888
	}
	a := fmt.Sprintf("%s:%d", h, p)
	s.logger.Info(fmt.Sprintf("starting server: %s", a))
	return s.echo.Start(a)
}
