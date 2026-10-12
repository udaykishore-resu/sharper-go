// Package obs is Technique 85: one structured logger, built in
// one place, and types that control how they appear.
package obs

import (
	"io"
	"log/slog"
)

var Level = new(slog.LevelVar) // Info by default

func NewLogger(w io.Writer, ver string) *slog.Logger {
	h := slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: Level,
	})
	return slog.New(h).With(
		slog.String("service", "ledgerd"),
		slog.String("version", ver))
}

type Transfer struct {
	ID, From, To string
	Amount       int64
	Key          string
}

func (t Transfer) LogValue() slog.Value {
	return slog.GroupValue(
		slog.String("id", t.ID),
		slog.Int64("amount", t.Amount))
}
