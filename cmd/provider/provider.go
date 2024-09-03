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
type ContextLogger struct{}

func main() {
	err := (&cli.App{
		Before: func(c *cli.Context) error {
			logger, err := configureLogging(c.String("log-level"))
			if err != nil {
				return err
			}
			c.Context = context.WithValue(c.Context, ContextLogger{}, logger)
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
					var err error
					l, ok := c.Context.Value(ContextLogger{}).(*slog.Logger)
					if !ok {
						return fmt.Errorf("logger not attached to context")
					}

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

					s, err := provider.New(&provider.Opts{
						FilterExclude:      c.StringSlice("filter-exclude"),
						FilterInclude:      c.StringSlice("filter-include"),
						FilterRegexExclude: fre,
						FilterRegexInclude: fri,
						Logger:             l,
						RouterOSAddress:    c.String("routeros-address"),
						RouterOSPassword:   c.String("routeros-password"),
						RouterOSUsername:   c.String("routeros-username"),
						ServerHost:         c.String("server-host"),
						ServerPort:         c.Uint("server-port"),
					})

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
