package observabilitydemo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/trace"

	"gk/internal/platform/httpserver"
)

var (
	errOutOfStock         = errors.New("out of stock")
	errPaymentDeclined    = errors.New("payment declined")
	errPaymentUnavailable = errors.New("payment unavailable")
)

type paymentRequest struct {
	OrderID     string `json:"orderId"`
	AmountCents int    `json:"amountCents"`
	Attempt     int    `json:"attempt"`
	Failures    int    `json:"failures"`
	DelayMS     int    `json:"delayMs"`
	Declined    bool   `json:"declined"`
}

type paymentSimulator struct {
	server    *http.Server
	client    *http.Client
	transport *http.Transport
	url       string
}

// The simulator listens only on an OS-assigned loopback port. No configurable
// payment URL, credentials, real provider, or persistent payment state exists.
func startPaymentSimulator(logger *slog.Logger) (*paymentSimulator, error) {
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// Never send simulated payment payloads through an environment HTTP proxy.
	transport.Proxy = nil
	simulator := &paymentSimulator{transport: transport, url: "http://" + listener.Addr().String() + "/api/v1/demo/payments/authorize"}
	simulator.client = &http.Client{Transport: otelhttp.NewTransport(transport), Timeout: 5 * time.Second}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/v1/demo/payments/authorize", func(w http.ResponseWriter, r *http.Request) {
		var body paymentRequest
		if err := httpserver.DecodeJSON(w, r, &body); err != nil || body.DelayMS < 0 || body.DelayMS > 3000 || body.Attempt < 1 || body.Attempt > 4 || body.Failures < 0 || body.Failures > 3 || body.OrderID == "" || body.AmountCents <= 0 {
			httpserver.WriteError(w, r, 400, "invalid_payment", "invalid simulated payment")
			return
		}
		if err := pause(r.Context(), time.Duration(body.DelayMS)*time.Millisecond); err != nil {
			return
		}
		switch {
		case body.Attempt <= body.Failures:
			logger.WarnContext(r.Context(), "mock payment temporarily unavailable", "order_id", body.OrderID, "attempt", body.Attempt)
			httpserver.WriteError(w, r, 503, "provider_unavailable", "simulated provider outage")
		case body.Declined:
			logger.InfoContext(r.Context(), "mock payment declined", "order_id", body.OrderID)
			httpserver.WriteError(w, r, 402, "card_declined", "simulated card declined")
		default:
			logger.InfoContext(r.Context(), "mock payment authorized", "order_id", body.OrderID, "amount_cents", body.AmountCents)
			httpserver.WriteJSON(w, 200, map[string]string{"authorizationId": "mock-" + body.OrderID})
		}
	})
	simulator.server = &http.Server{Handler: otelhttp.NewHandler(httpserver.RouteTelemetry(mux), "payment.mock", otelhttp.WithSpanNameFormatter(func(_ string, _ *http.Request) string { return "payment.mock" })), ReadHeaderTimeout: time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 5 * time.Second, IdleTimeout: 30 * time.Second}
	go func() {
		if err := simulator.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Error("mock payment server failed", "error", err)
		}
	}()
	return simulator, nil
}

func (s *paymentSimulator) authorize(ctx context.Context, body paymentRequest) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Idempotency-Key", body.OrderID)
	response, err := s.client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	// Read the response fully so the connection is reusable and client span ends.
	data, err := io.ReadAll(io.LimitReader(response.Body, 4096))
	if err != nil {
		return err
	}
	switch response.StatusCode {
	case 200:
		var result struct {
			AuthorizationID string `json:"authorizationId"`
		}
		if json.Unmarshal(data, &result) != nil || result.AuthorizationID != "mock-"+body.OrderID {
			return errPaymentUnavailable
		}
		trace.SpanFromContext(ctx).AddEvent("payment authorized")
		return nil
	case 402:
		return errPaymentDeclined
	default:
		return fmt.Errorf("%w: HTTP %d", errPaymentUnavailable, response.StatusCode)
	}
}

func (s *paymentSimulator) close(ctx context.Context) error {
	s.transport.CloseIdleConnections()
	err := s.server.Shutdown(ctx)
	if err != nil {
		_ = s.server.Close()
	}
	return err
}
