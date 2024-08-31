package provider

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"sigs.k8s.io/external-dns/endpoint"
	"sigs.k8s.io/external-dns/plan"
	ednsprovider "sigs.k8s.io/external-dns/provider"
)

// Defines a provider interface - extending that defined by [ednsprovider.Provider]
type Provider interface {
	ednsprovider.Provider
	Health() error
}

// Internal configuration and state of a provider struct
type provider struct {
	client       Client
	domainFilter endpoint.DomainFilter
	logger       *slog.Logger
}

// Options used when constructing a new provider
type ProviderOpts struct {
	DomainFilter endpoint.DomainFilter
	Client       Client
	Logger       *slog.Logger
}

// Creates a new [provider] using the provided options within [ProviderOpts]
func NewProvider(o *ProviderOpts) (*provider, error) {
	l := o.Logger
	if l == nil {
		l = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	return &provider{
		client:       o.Client,
		domainFilter: o.DomainFilter,
		logger:       l,
	}, nil
}

// According to [ednsprovider.Provider], 'canonicalizes' endpoints to be consistent with that of the provider.
// Currently, this provider does not need to canonicalize endpoints and simply returns the given input.
func (p *provider) AdjustEndpoints(es []*endpoint.Endpoint) ([]*endpoint.Endpoint, error) {
	return es, nil
}

// Applies DNS changes to the target using this provider.
// Returns an error if any update operation fails.
// Attempts to apply all changes before returning an error on failure.
func (p *provider) ApplyChanges(co context.Context, ch *plan.Changes) error {
	p.logger.Info("applying changes")

	errs := []error{}

	for _, e := range append(ch.Delete, ch.UpdateOld...) {
		p.logger.Info(fmt.Sprintf("deleting record %s %s", e.RecordType, e.DNSName))
		err := p.client.DeleteEndpoint(e)
		if err != nil {
			p.logger.Warn(fmt.Sprintf("failed to delete record %s %s: %s", e.RecordType, e.DNSName, err.Error()))
			errs = append(errs, err)
		}
	}

	for _, e := range append(ch.Create, ch.UpdateNew...) {
		p.logger.Info(fmt.Sprintf("creating record %s %s", e.RecordType, e.DNSName))
		err := p.client.CreateEndpoint(e)
		if err != nil {
			p.logger.Error(fmt.Sprintf("failed to create record %s %s: %s", e.RecordType, e.DNSName, err.Error()))
			errs = append(errs, err)
		}
	}

	if len(errs) != 0 {
		return fmt.Errorf("failed to update %d records", len(errs))
	}

	return nil
}

// Performs a health check of provider and client
// Returns an error if the provider/client are unhealthy
func (p *provider) Health() error {
	p.logger.Info("performing health check")
	err := p.client.Health()
	if err != nil {
		err = fmt.Errorf("client health check failed: %w", err)
	}
	return err
}

// Gets the domain filters configured when the provider was launched
func (p *provider) GetDomainFilter() endpoint.DomainFilter {
	return p.domainFilter
}

// Gets known records attached to this provider
func (p *provider) Records(c context.Context) ([]*endpoint.Endpoint, error) {
	p.logger.Info("fetching records")
	return p.client.ListEndpoints()
}
