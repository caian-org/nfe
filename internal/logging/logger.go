// Package logging builds a structured logger that writes to stderr so it does
// not interleave with command stdout (which is reserved for the renderer).
package logging

import (
	"io"
	"log/slog"
	"os"
)

// New returns a slog.Logger writing to stderr (or w if non-nil). Text handler
// is the default; JSON handler is selected when jsonOutput is true so that the
// log lines remain machine-parsable alongside the renderer's JSON output.
func New(jsonOutput bool, level slog.Level, w io.Writer) *slog.Logger {
	if w == nil {
		w = os.Stderr
	}
	opts := &slog.HandlerOptions{Level: level}
	var h slog.Handler
	if jsonOutput {
		h = slog.NewJSONHandler(w, opts)
	} else {
		h = slog.NewTextHandler(w, opts)
	}
	return slog.New(h)
}
