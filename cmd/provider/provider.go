package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"regexp"
	"strings"

	"github.com/benfiola/external-dns-routeros-provider/internal/provider"
	"github.com/urfave/cli/v2"
	"sigs.k8s.io/external-dns/endpoint"
)

// Configures logging for the application.
// Accepts a logging level 'error' | 'warn' | 'info' | 'debug'
func configureLogging(ls string) (*slog.Logger, error) {
	if ls == "" {
		ls = "info"
	}
	var l slog.Level
	switch ls {
	case "error":
		l = slog.LevelError
	case "warn":
		l = slog.LevelWarn
	case "info":
		l = slog.LevelInfo
	case "debug":
		l = slog.LevelDebug
	default:
		return nil, fmt.Errorf("unrecognized log level %s", ls)
	}

	opts := &slog.HandlerOptions{
		Level: l,
	}
	handler := slog.NewTextHandler(os.Stderr, opts)
	logger := slog.New(handler)
	return logger, nil
}

// Used as a key to the urfave/cli context to store the application-level logger.
type Logger struct{}

func main() {
	err := (&cli.App{
		Before: func(c *cli.Context) error {
			logger, err := configureLogging(c.String("log-level"))
			if err != nil {
				return err
			}
			c.Context = context.WithValue(c.Context, Logger{}, logger)
			return nil
		},
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "log-level",
				Usage: "logging verbosity level",
			},
		},
		Commands: []*cli.Command{
			{
				Name:  "run",
				Usage: "start provider webhook server",
				Flags: []cli.Flag{
					&cli.StringSliceFlag{
						Name:    "filter-exclude",
						Usage:   "dns string exclusion filter",
						EnvVars: []string{"EXTERNAL_DNS_ROUTEROS_PROVIDER_FILTER_EXCLUDE"},
					},
					&cli.StringSliceFlag{
						Name:    "filter-include",
						Usage:   "dns string inclusion filter",
						EnvVars: []string{"EXTERNAL_DNS_ROUTEROS_PROVIDER_FILTER_INCLUDE"},
					},
					&cli.StringFlag{
						Name:    "filter-regex-exclude",
						Usage:   "dns regex exclusion filter",
						EnvVars: []string{"EXTERNAL_DNS_ROUTEROS_PROVIDER_FILTER_REGEX_EXCLUDE"},
					},
					&cli.StringFlag{
						Name:    "filter-regex-include",
						Usage:   "dns regex inclusion filter",
						EnvVars: []string{"EXTERNAL_DNS_ROUTEROS_PROVIDER_FILTER_REGEX_INCLUDE"},
					},
					&cli.StringFlag{
						Name:    "routeros-address",
						Usage:   "routeros address (<host>:<port>)",
						EnvVars: []string{"EXTERNAL_DNS_ROUTEROS_PROVIDER_ROUTEROS_ADDRESS"},
					},
					&cli.StringFlag{
						Name:    "routeros-password",
						Usage:   "routeros password",
						EnvVars: []string{"EXTERNAL_DNS_ROUTEROS_PROVIDER_ROUTEROS_PASSWORD"},
					},
					&cli.StringFlag{
						Name:    "routeros-username",
						Usage:   "routeros username",
						EnvVars: []string{"EXTERNAL_DNS_ROUTEROS_PROVIDER_ROUTEROS_USERNAME"},
					},
					&cli.StringFlag{
						Name:    "server-host",
						Usage:   "host to bind to",
						EnvVars: []string{"EXTERNAL_DNS_ROUTEROS_PROVIDER_SERVER_HOST"},
						Value:   "127.0.0.1",
					},
					&cli.UintFlag{
						Name:    "server-port",
						Usage:   "port to bind to",
						EnvVars: []string{"EXTERNAL_DNS_ROUTEROS_PROVIDER_SERVER_PORT"},
						Value:   8888,
					},
				},
				Action: func(c *cli.Context) error {
					l, ok := c.Context.Value(Logger{}).(*slog.Logger)
					if !ok {
						return fmt.Errorf("logger not attached to context")
					}

					ra := c.String("routeros-address")
					rp := c.String("routeros-password")
					ru := c.String("routeros-username")
					pc, err := provider.NewClient(provider.ClientOpts{
						Address:  ra,
						Logger:   l.With("name", "client"),
						Password: rp,
						Username: ru,
					})
					if err != nil {
						return err
					}

					fe := c.StringSlice("filter-exclude")
					fi := c.StringSlice("filter-include")
					var fre *regexp.Regexp
					if fres := c.String("filter-regex-exclude"); fres != "" {
						fre, err = regexp.Compile(fres)
						if err != nil {
							return err
						}
					}
					var fri *regexp.Regexp
					if fris := c.String("filter-regex-include"); fris != "" {
						fri, err = regexp.Compile(fris)
						if err != nil {
							return err
						}
					}
					df := endpoint.DomainFilter{}
					if fe != nil || fi != nil {
						df = endpoint.NewDomainFilterWithExclusions(fi, fe)
					} else if fre != nil || fri != nil {
						df = endpoint.NewRegexDomainFilter(fri, fre)
					}
					p, err := provider.NewProvider(provider.ProviderOpts{
						Client:       pc,
						DomainFilter: df,
						Logger:       l.With("name", "provider"),
					})
					if err != nil {
						return err
					}

					sh := c.String("server-host")
					sp := c.Uint("server-port")
					s, err := provider.NewServer(&provider.ServerOpts{
						Host:     sh,
						Logger:   l.With("name", "server"),
						Port:     sp,
						Provider: p,
					})
					if err != nil {
						return err
					}
					return s.Run()
				},
			},
			{
				Name:  "version",
				Usage: "prints the provider version",
				Action: func(c *cli.Context) error {
					v := strings.TrimSpace(provider.ProviderVersion)
					fmt.Fprintf(c.App.Writer, "%s", v)
					return nil
				},
			},
		},
	}).Run(os.Args)
	code := 0
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err.Error())
		code = 1
	}
	os.Exit(code)
}
