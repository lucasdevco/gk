package httpserver

import (
	"net/http"
	"strconv"
	"time"
)

// ServerTiming measures time until final response headers are committed.
// It does not buffer bodies or include body transmission time.
func ServerTiming(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := &timingWriter{ResponseWriter: w, started: time.Now()}
		next.ServeHTTP(response, r)
		if !response.committed {
			response.WriteHeader(http.StatusOK)
		}
	})
}

type timingWriter struct {
	http.ResponseWriter
	started   time.Time
	committed bool
}

func (w *timingWriter) WriteHeader(status int) {
	if w.committed {
		return
	}
	if status >= 200 || status == http.StatusSwitchingProtocols {
		milliseconds := float64(time.Since(w.started)) / float64(time.Millisecond)
		w.Header().Add("Server-Timing", "app;dur="+strconv.FormatFloat(milliseconds, 'f', 3, 64))
		w.committed = true
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *timingWriter) Write(body []byte) (int, error) {
	if !w.committed {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

func (w *timingWriter) FlushError() error {
	if !w.committed {
		w.WriteHeader(http.StatusOK)
	}
	return http.NewResponseController(w.ResponseWriter).Flush()
}

func (w *timingWriter) Flush() { _ = w.FlushError() }

func (w *timingWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
