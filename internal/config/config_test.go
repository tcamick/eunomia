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
	require.NoError(t, c.Validate("/work"))
	assert.Equal(t, filepath.Join("/work", "data"), c.DataDir, "a relative data dir resolves against cwd")
	assert.Equal(t, "127.0.0.1:8080", c.Listen)
	assert.Equal(t, "127.0.0.1:8081", c.InternalListen)
	assert.Empty(t, c.PushgatewayURL, "no push unless asked")
}

func TestBind_EnvironmentOverridesDefaultsAndFlagsOverrideEnvironment(t *testing.T) {
	t.Parallel()
	env := map[string]string{EnvListen: "0.0.0.0:9000", EnvLogLevel: "debug"}
	c := parse(t, env, "-log-level=warn")
	require.NoError(t, c.Validate("/"))
	assert.Equal(t, "0.0.0.0:9000", c.Listen, "the environment sets the default")
	assert.Equal(t, "warn", c.LogLevel, "a flag beats the environment")
}

func TestValidate_ReportsEveryProblemAtOnce(t *testing.T) {
	t.Parallel()
	c := parse(t, nil,
		"-data-dir=", "-listen=nope", "-internal-listen=:0",
		"-log-level=loud", "-log-format=xml", "-pushgateway-url=ftp://x")
	err := c.Validate("/")
	require.Error(t, err)
	for _, flagName := range []string{"-data-dir", "-listen", "-internal-listen", "-log-level", "-log-format", "-pushgateway-url"} {
		assert.Contains(t, err.Error(), flagName+":", "expected a problem naming %s", flagName)
	}
}

func TestValidate_RejectsMetricsOnTheAppPort(t *testing.T) {
	t.Parallel()
	c := parse(t, nil, "-listen=0.0.0.0:8080", "-internal-listen=0.0.0.0:8080")
	err := c.Validate("/")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must differ from -listen")
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
