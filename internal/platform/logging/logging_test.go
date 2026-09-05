package logging

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

func TestLogColor(t *testing.T) {
	for _, tc := range []struct {
		level slog.Level
		color string
	}{
		{slog.LevelDebug, "\x1b[36m"},
		{slog.LevelInfo, "\x1b[32m"},
		{slog.LevelWarn, "\x1b[33m"},
		{slog.LevelError, "\x1b[31m"},
	} {
		t.Run(tc.level.String(), func(t *testing.T) {
			var output bytes.Buffer
			logger := newLogger(&output, "debug", "text", "test", true)
			logger.Log(context.Background(), tc.level, "hello", "key", "value")
			want := tc.color + "level=" + tc.level.String() + "\x1b[0m"
			if !strings.Contains(output.String(), want) || !strings.Contains(output.String(), "key=value") {
				t.Fatalf("unexpected output: %q", output.String())
			}
		})
	}
}

func TestPlainAndJSONLogs(t *testing.T) {
	for _, tc := range []struct {
		format string
		color  bool
	}{
		{"text", false}, {"json", false}, {"json", true},
	} {
		var output bytes.Buffer
		logger := newLogger(&output, "info", tc.format, "test", tc.color)
		logger.Debug("filtered")
		logger.Info("hello")
		if strings.Contains(output.String(), "\x1b") || strings.Contains(output.String(), "filtered") {
			t.Fatalf("unexpected output: %q", output.String())
		}
		if tc.format == "json" && !json.Valid(output.Bytes()) {
			t.Fatalf("invalid JSON: %q", output.String())
		}
	}
}

func TestColorPreservesTraceAndAttributes(t *testing.T) {
	var output bytes.Buffer
	span := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{1}, SpanID: trace.SpanID{2},
	})
	ctx := trace.ContextWithSpanContext(context.Background(), span)
	logger := newLogger(&output, "info", "text", "test", true).
		With("request_id", "req-1").WithGroup("operation").With("name", "create")
	logger.InfoContext(ctx, "hello")
	for _, want := range []string{
		"service=gk", "environment=test", "request_id=req-1", "operation.name=create",
		"operation.trace_id=" + span.TraceID().String(),
		"operation.span_id=" + span.SpanID().String(),
	} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("missing %q in %q", want, output.String())
		}
	}
}
