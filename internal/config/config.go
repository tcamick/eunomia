// Package config parses and validates every runtime setting at startup, so a
// bad value stops the process with a message naming the flag instead of
// surfacing as a confusing failure inside a job hours later.
package config

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
)

// Config holds every setting after flags and environment overrides apply.
type Config struct {
	// DataDir holds the SQLite file and, from E1a, mirrored portraits.
	DataDir string
	// Listen is the host:port of the app and its JSON API, which the
	// tailnet reaches through the Tailscale proxy.
	Listen string
	// InternalListen is the host:port of /metrics, which only Prometheus
	// reaches (k8s/app/network.yaml). It must differ from Listen.
	InternalListen string
	// LogLevel is one of debug, info, warn, error.
	LogLevel string
	// LogFormat is json or text.
	LogFormat string
	// PushgatewayURL, when set, is where a job pushes its counters once at
	// exit, since its pod is gone before Prometheus could scrape it
	// (beholder ADR 0002). Empty (a laptop) pushes nothing.
	PushgatewayURL string
	// LegislatorsURL is the congress-legislators dataset's directory, and
	// PortraitsURL the portraits' (go-conventions, Fetching: a source's base
	// URL is a flag). Both are https and end in "/".
	LegislatorsURL string
	PortraitsURL   string
}

// Environment variables that override each flag's default.
const (
	EnvDataDir        = "EUNOMIA_DATA_DIR"
	EnvListen         = "EUNOMIA_LISTEN"
	EnvInternalListen = "EUNOMIA_INTERNAL_LISTEN"
	EnvLogLevel       = "LOG_LEVEL"
	EnvLogFormat      = "LOG_FORMAT"
	EnvPushgatewayURL = "EUNOMIA_PUSHGATEWAY_URL"
	EnvLegislatorsURL = "EUNOMIA_LEGISLATORS_URL"
	EnvPortraitsURL   = "EUNOMIA_PORTRAITS_URL"
)

// The sources' published directories, the URL flags' defaults.
const (
	DefaultLegislatorsURL = "https://unitedstates.github.io/congress-legislators/"
	DefaultPortraitsURL   = "https://unitedstates.github.io/images/congress/225x275/"
)

// Bind registers every flag on fs with its default taken from getenv when
// the variable is set, and returns the Config the flags write into. Call
// fs.Parse, then Validate.
func Bind(fs *flag.FlagSet, getenv func(string) string) *Config {
	c := &Config{}
	def := func(env, fallback string) string {
		if v := getenv(env); v != "" {
			return v
		}
		return fallback
	}
	fs.StringVar(&c.DataDir, "data-dir", def(EnvDataDir, "data"),
		"directory for the SQLite file and mirrored files; created if missing (env "+EnvDataDir+")")
	fs.StringVar(&c.Listen, "listen", def(EnvListen, "127.0.0.1:8080"),
		"host:port for the app and its API (env "+EnvListen+")")
	fs.StringVar(&c.InternalListen, "internal-listen", def(EnvInternalListen, "127.0.0.1:8081"),
		"host:port for /metrics (env "+EnvInternalListen+")")
	fs.StringVar(&c.LogLevel, "log-level", def(EnvLogLevel, "info"),
		"debug, info, warn or error (env "+EnvLogLevel+")")
	fs.StringVar(&c.LogFormat, "log-format", def(EnvLogFormat, "json"),
		"json or text (env "+EnvLogFormat+")")
	fs.StringVar(&c.PushgatewayURL, "pushgateway-url", def(EnvPushgatewayURL, ""),
		"Pushgateway base URL a job pushes its counters to at exit; empty pushes nothing (env "+EnvPushgatewayURL+")")
	fs.StringVar(&c.LegislatorsURL, "legislators-url", def(EnvLegislatorsURL, DefaultLegislatorsURL),
		"https directory of the congress-legislators JSON files, ending in / (env "+EnvLegislatorsURL+")")
	fs.StringVar(&c.PortraitsURL, "portraits-url", def(EnvPortraitsURL, DefaultPortraitsURL),
		"https directory of the members' portraits, ending in / (env "+EnvPortraitsURL+")")
	return c
}

// Validate checks every setting and reports every problem at once, each
// naming its flag. A relative DataDir is resolved against cwd.
func (c *Config) Validate(cwd string) error {
	var errs []error
	if strings.TrimSpace(c.DataDir) == "" {
		errs = append(errs, errors.New("-data-dir: must not be empty"))
	} else if !filepath.IsAbs(c.DataDir) {
		c.DataDir = filepath.Join(cwd, c.DataDir)
	}
	if err := validAddr(c.Listen); err != nil {
		errs = append(errs, fmt.Errorf("-listen: %w", err))
	}
	if err := validAddr(c.InternalListen); err != nil {
		errs = append(errs, fmt.Errorf("-internal-listen: %w", err))
	}
	if sameAddr(c.Listen, c.InternalListen) {
		errs = append(errs, errors.New("-internal-listen: must differ from -listen, or /metrics would reach the tailnet"))
	}
	switch strings.ToLower(c.LogLevel) {
	case "debug", "info", "warn", "error":
	default:
		errs = append(errs, fmt.Errorf("-log-level: %q is not one of debug, info, warn, error", c.LogLevel))
	}
	switch strings.ToLower(c.LogFormat) {
	case "json", "text":
	default:
		errs = append(errs, fmt.Errorf("-log-format: %q is not json or text", c.LogFormat))
	}
	if c.PushgatewayURL != "" {
		u, err := url.Parse(c.PushgatewayURL)
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			errs = append(errs, fmt.Errorf("-pushgateway-url: %q is not an http or https URL with a host", c.PushgatewayURL))
		}
	}
	for _, src := range []struct{ flag, val string }{
		{"-legislators-url", c.LegislatorsURL}, {"-portraits-url", c.PortraitsURL},
	} {
		u, err := url.Parse(src.val)
		if err != nil || u.Scheme != "https" || u.Host == "" || !strings.HasSuffix(u.Path, "/") || u.RawQuery != "" {
			errs = append(errs, fmt.Errorf("%s: %q is not an https URL with a host and a path ending in /", src.flag, src.val))
		}
	}
	return errors.Join(errs...)
}

// sameAddr reports whether two listen addresses would bind the same port: the
// same host, or the same port where either host means every interface.
func sameAddr(a, b string) bool {
	ha, pa, errA := net.SplitHostPort(a)
	hb, pb, errB := net.SplitHostPort(b)
	if errA != nil || errB != nil || pa != pb {
		return false
	}
	wildcard := func(h string) bool { return h == "" || h == "0.0.0.0" || h == "::" }
	return ha == hb || wildcard(ha) || wildcard(hb)
}

// validAddr accepts host:port with a numeric port from 1 to 65535; the host
// may be empty (all interfaces).
func validAddr(addr string) error {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("%q is not host:port: %w", addr, err)
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("%q: port must be a number from 1 to 65535", addr)
	}
	return nil
}
