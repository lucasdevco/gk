// Package logging configures structured process logging.
package logging

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"os"
	"strings"

	"go.opentelemetry.io/otel/trace"
)

func New(level, format, environment string, color bool) *slog.Logger {
	return newLogger(os.Stdout, level, format, environment, color)
}

func newLogger(output io.Writer, level, format, environment string, color bool) *slog.Logger {
	options := &slog.HandlerOptions{Level: parseLevel(level)}
	var handler slog.Handler
	if format == "json" {
		handler = slog.NewJSONHandler(output, options)
	} else {
		if color {
			output = colorWriter{output}
		}
		handler = slog.NewTextHandler(output, options)
	}
	return slog.New(&traceHandler{Handler: handler}).With("service", "gk", "environment", environment)
}

// colorWriter colors only the built-in level field. Decorating the serialized
// text keeps ANSI escapes out of slog's quoted attribute values.
type colorWriter struct{ io.Writer }

func (w colorWriter) Write(p []byte) (int, error) {
	start := bytes.Index(p, []byte(" level="))
	if start < 0 {
		return w.Writer.Write(p)
	}
	start++
	end := bytes.IndexByte(p[start:], ' ')
	if end < 0 {
		return w.Writer.Write(p)
	}
	end += start
	var color string
	switch string(p[start:end]) {
	case "level=DEBUG":
		color = "\x1b[36m"
	case "level=INFO":
		color = "\x1b[32m"
	case "level=WARN":
		color = "\x1b[33m"
	case "level=ERROR":
		color = "\x1b[31m"
	default:
		return w.Writer.Write(p)
	}
	line := make([]byte, 0, len(p)+len(color)+4)
	line = append(line, p[:start]...)
	line = append(line, color...)
	line = append(line, p[start:end]...)
	line = append(line, "\x1b[0m"...)
	line = append(line, p[end:]...)
	n, err := w.Writer.Write(line)
	if err == nil && n != len(line) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return 0, err
	}
	return len(p), nil
}

func parseLevel(value string) slog.Level {
	switch strings.ToLower(value) {
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

type traceHandler struct{ slog.Handler }

func (h *traceHandler) Handle(ctx context.Context, record slog.Record) error {
	span := trace.SpanContextFromContext(ctx)
	if span.IsValid() {
		record.AddAttrs(
			slog.String("trace_id", span.TraceID().String()),
			slog.String("span_id", span.SpanID().String()),
		)
	}
	return h.Handler.Handle(ctx, record)
}

func (h *traceHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &traceHandler{Handler: h.Handler.WithAttrs(attrs)}
}

func (h *traceHandler) WithGroup(name string) slog.Handler {
	return &traceHandler{Handler: h.Handler.WithGroup(name)}
}
