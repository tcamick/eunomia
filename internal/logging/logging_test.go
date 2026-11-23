package logging

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_WritesJSONAtTheChosenLevel(t *testing.T) {
	t.Parallel()
	tests := map[string]struct {
		level     string
		wantDebug bool
	}{
		"debug shows debug lines":             {level: "debug", wantDebug: true},
		"info hides debug lines":              {level: "info", wantDebug: false},
		"an unknown level falls back to info": {level: "loud", wantDebug: false},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var buf bytes.Buffer
			log := New(&buf, tc.level, "json")
			log.Debug().Msg("Checked debug")
			log.Info().Str("job", "noop").Msg("Checked info")
			lines := bytes.Split(bytes.TrimSpace(buf.Bytes()), []byte("\n"))
			if tc.wantDebug {
				require.Len(t, lines, 2, "expected the debug and the info line")
			} else {
				require.Len(t, lines, 1, "expected only the info line")
			}
			var last map[string]any
			require.NoError(t, json.Unmarshal(lines[len(lines)-1], &last), "the info line should be JSON")
			assert.Equal(t, "Checked info", last["message"])
			assert.Equal(t, "noop", last["job"])
		})
	}
}

func TestNew_TextFormatHasNoColourWhenNotATerminal(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	log := New(&buf, "info", "text")
	log.Info().Msg("Started server")
	assert.Contains(t, buf.String(), "Started server")
	assert.NotContains(t, buf.String(), "\x1b[", "a buffer is not a terminal, so no escape sequences")
}
