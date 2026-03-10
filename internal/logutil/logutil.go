package logutil

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/SavingFrame/spotisleep/internal/domain"
)

func Configure() {
	slog.SetDefault(slog.New(newHumanHandler(os.Stderr, parseLevel(os.Getenv("LOG_LEVEL")))))
}

func Song(song *domain.Song) slog.Value {
	return slog.StringValue(SongRef(song))
}

func SongRef(song *domain.Song) string {
	if song == nil {
		return "<nil song>"
	}
	artists := strings.Join(song.Artists, ", ")
	if artists == "" {
		return song.Title
	}
	if song.Title == "" {
		return artists
	}
	return fmt.Sprintf("%s — %s", artists, song.Title)
}

func ShortToken(token string) string {
	if len(token) <= 10 {
		return token
	}
	return token[:6] + "…" + token[len(token)-4:]
}

func parseLevel(raw string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

type humanHandler struct {
	w      io.Writer
	level  slog.Level
	attrs  []slog.Attr
	groups []string
	mu     *sync.Mutex
}

func newHumanHandler(w io.Writer, level slog.Level) *humanHandler {
	return &humanHandler{
		w:     w,
		level: level,
		mu:    &sync.Mutex{},
	}
}

func (h *humanHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= h.level
}

func (h *humanHandler) Handle(_ context.Context, record slog.Record) error {
	allAttrs := make([]slog.Attr, 0, len(h.attrs)+record.NumAttrs())
	allAttrs = append(allAttrs, h.attrs...)
	record.Attrs(func(attr slog.Attr) bool {
		allAttrs = append(allAttrs, attr)
		return true
	})

	parts := []string{record.Time.Format("15:04:05"), levelLabel(record.Level), record.Message}
	for _, attr := range allAttrs {
		h.appendAttr(&parts, h.groups, attr)
	}
	line := strings.Join(parts, " | ") + "\n"

	h.mu.Lock()
	defer h.mu.Unlock()
	_, err := io.WriteString(h.w, line)
	return err
}

func (h *humanHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &humanHandler{
		w:      h.w,
		level:  h.level,
		attrs:  append(append([]slog.Attr{}, h.attrs...), attrs...),
		groups: append([]string{}, h.groups...),
		mu:     h.mu,
	}
}

func (h *humanHandler) WithGroup(name string) slog.Handler {
	return &humanHandler{
		w:      h.w,
		level:  h.level,
		attrs:  append([]slog.Attr{}, h.attrs...),
		groups: append(append([]string{}, h.groups...), name),
		mu:     h.mu,
	}
}

func (h *humanHandler) appendAttr(parts *[]string, groups []string, attr slog.Attr) {
	attr.Value = attr.Value.Resolve()
	if attr.Equal(slog.Attr{}) {
		return
	}

	key := attr.Key
	if len(groups) > 0 {
		key = strings.Join(append(append([]string{}, groups...), key), ".")
	}

	switch attr.Value.Kind() {
	case slog.KindGroup:
		nextGroups := groups
		if attr.Key != "" {
			nextGroups = append(append([]string{}, groups...), attr.Key)
		}
		for _, nested := range attr.Value.Group() {
			h.appendAttr(parts, nextGroups, nested)
		}
	default:
		*parts = append(*parts, fmt.Sprintf("%s: %s", key, formatValue(attr.Value)))
	}
}

func formatValue(v slog.Value) string {
	switch v.Kind() {
	case slog.KindString:
		return v.String()
	case slog.KindBool:
		return strconv.FormatBool(v.Bool())
	case slog.KindInt64:
		return strconv.FormatInt(v.Int64(), 10)
	case slog.KindUint64:
		return strconv.FormatUint(v.Uint64(), 10)
	case slog.KindFloat64:
		return strconv.FormatFloat(v.Float64(), 'f', 1, 64)
	case slog.KindDuration:
		return v.Duration().String()
	case slog.KindTime:
		return v.Time().Format(time.RFC3339)
	case slog.KindAny:
		return fmt.Sprint(v.Any())
	default:
		return v.String()
	}
}

func levelLabel(level slog.Level) string {
	switch {
	case level >= slog.LevelError:
		return "ERROR"
	case level >= slog.LevelWarn:
		return "WARN"
	case level >= slog.LevelInfo:
		return "INFO"
	default:
		return "DEBUG"
	}
}
