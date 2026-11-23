// Package logging builds the process logger. Callers put it in a context with
// logger.WithContext and packages retrieve it with zerolog.Ctx(ctx), so there
// is no package-level mutable logger.
package logging

import (
	"io"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

// New returns a logger at level writing format ("json" or "text") to w. Text
// is for a person at a terminal; it is coloured only when w is one, so a
// redirected or piped text log carries no escape sequences. An unknown level
// falls back to info: config.Validate has already rejected it, so this is
// defensive, not a second validation.
func New(w io.Writer, level, format string) zerolog.Logger {
	lvl, err := zerolog.ParseLevel(strings.ToLower(level))
	if err != nil || lvl == zerolog.NoLevel {
		lvl = zerolog.InfoLevel
	}
	if strings.EqualFold(format, "text") {
		w = zerolog.ConsoleWriter{Out: w, TimeFormat: time.RFC3339, NoColor: !isTerminal(w)}
	}
	return zerolog.New(w).Level(lvl).With().Timestamp().Logger()
}

// isTerminal reports whether w is a character device such as a tty.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
