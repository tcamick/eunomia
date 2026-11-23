package config

import (
	"flag"
	"io"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parse binds a fresh flag set with env as the environment and parses args.
func parse(t *testing.T, env map[string]string, args ...string) *Config {
	t.Helper()
	fs := flag.NewFlagSet("test", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	c := Bind(fs, func(k string) string { return env[k] })
	require.NoError(t, fs.Parse(args), "parsing %v", args)
	return c
}

func TestValidate_DefaultsAreValid(t *testing.T) {
	t.Parallel()
	c := parse(t, nil)
	require.NoError(t, c.Validate("/work"), "calling c.Validate")
	assert.Equal(t, filepath.Join("/work", "data"), c.DataDir, "a relative data dir resolves against cwd")
	assert.Equal(t, "127.0.0.1:8080", c.Listen)
	assert.Equal(t, "127.0.0.1:8081", c.InternalListen)
	assert.Empty(t, c.PushgatewayURL, "no push unless asked")
	assert.Equal(t, "https://unitedstates.github.io/congress-legislators/", c.LegislatorsURL, "the dataset's published directory")
	assert.Equal(t, "https://unitedstates.github.io/images/congress/225x275/", c.PortraitsURL, "the portraits' published directory")
}

func TestBind_EnvironmentOverridesDefaultsAndFlagsOverrideEnvironment(t *testing.T) {
	t.Parallel()
	env := map[string]string{EnvListen: "0.0.0.0:9000", EnvLogLevel: "debug"}
	c := parse(t, env, "-log-level=warn")
	require.NoError(t, c.Validate("/"), "calling c.Validate")
	assert.Equal(t, "0.0.0.0:9000", c.Listen, "the environment sets the default")
	assert.Equal(t, "warn", c.LogLevel, "a flag beats the environment")
}

func TestValidate_ReportsEveryProblemAtOnce(t *testing.T) {
	t.Parallel()
	c := parse(t, nil,
		"-data-dir=", "-listen=nope", "-internal-listen=:0",
		"-log-level=loud", "-log-format=xml", "-pushgateway-url=ftp://x",
		"-legislators-url=http://example.org/", "-portraits-url=https://example.org/no-slash")
	err := c.Validate("/")
	require.Error(t, err, "expected c.Validate to fail")
	for _, flagName := range []string{"-data-dir", "-listen", "-internal-listen", "-log-level", "-log-format", "-pushgateway-url",
		"-legislators-url", "-portraits-url"} {
		assert.Contains(t, err.Error(), flagName+":", "expected a problem naming %s", flagName)
	}
}

func TestValidate_RejectsMetricsOnTheAppPort(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		listen, internal string
		clash            bool
	}{
		"the same address":                       {listen: "0.0.0.0:8080", internal: "0.0.0.0:8080", clash: true},
		"all interfaces written two ways":        {listen: ":8080", internal: "0.0.0.0:8080", clash: true},
		"one host and all interfaces, same port": {listen: "127.0.0.1:8080", internal: ":8080", clash: true},
		"two hosts, same port":                   {listen: "127.0.0.1:8080", internal: "10.0.0.1:8080", clash: false},
		"different ports":                        {listen: ":8080", internal: ":8081", clash: false},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c := parse(t, nil, "-listen="+tc.listen, "-internal-listen="+tc.internal)
			err := c.Validate("/")
			if tc.clash {
				require.Error(t, err, "expected %s and %s to clash", tc.listen, tc.internal)
				assert.Contains(t, err.Error(), "must differ from -listen")
			} else {
				assert.NoError(t, err, "expected %s and %s not to clash", tc.listen, tc.internal)
			}
		})
	}
}

func TestValidate_Addresses(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		addr string
		ok   bool
	}{
		"loopback with a port":         {addr: "127.0.0.1:8080", ok: true},
		"all interfaces with a port":   {addr: ":8080", ok: true},
		"a port above the range":       {addr: ":70000", ok: false},
		"a named port is not accepted": {addr: "localhost:http", ok: false},
		"no port at all":               {addr: "localhost", ok: false},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := validAddr(tc.addr)
			if tc.ok {
				assert.NoError(t, err, "addr %q", tc.addr)
			} else {
				assert.Error(t, err, "addr %q", tc.addr)
			}
		})
	}
}
