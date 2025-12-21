// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package lumigoreceiver // import "github.com/lumigo-io/lumigo-otel-collector-contrib/receiver/lumigoreceiver"

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/receiver"
	"go.uber.org/zap"
)

type lumigoReceiver struct {
	config       *Config
	consumer     consumer.Traces
	server       *http.Server
	settings     receiver.Settings
	shutdownChan chan struct{}
}

func newLumigoReceiver(
	config *Config,
	consumer consumer.Traces,
	settings receiver.Settings,
) (*lumigoReceiver, error) {
	if consumer == nil {
		return nil, errors.New("nil consumer")
	}

	r := &lumigoReceiver{
		config:       config,
		consumer:     consumer,
		settings:     settings,
		shutdownChan: make(chan struct{}),
	}

	return r, nil
}

func (r *lumigoReceiver) Start(ctx context.Context, host component.Host) error {
	endpoint := r.config.Endpoint
	if endpoint == "" {
		endpoint = "0.0.0.0:8088"
	}
	// Ensure ServerConfig uses the effective endpoint.
	r.config.Endpoint = endpoint

	r.settings.Logger.Info("Starting Lumigo receiver", zap.String("endpoint", endpoint))

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/traces", r.handleTraces)
	mux.HandleFunc("/health", r.handleHealth)

	var err error
	r.server, err = r.config.ServerConfig.ToServer(
		ctx,
		host,
		r.settings.TelemetrySettings,
		mux,
	)
	if err != nil {
		return fmt.Errorf("failed to create server: %w", err)
	}

	ln, err := r.config.ServerConfig.ToListener(ctx)
	if err != nil {
		return fmt.Errorf("failed to create listener: %w", err)
	}

	// Start the server in a goroutine
	go func() {
		// NOTE: confighttp.ServerConfig.ToServer() does not set http.Server.Addr.
		// We must Serve() on a listener created from ServerConfig, otherwise ListenAndServe()
		// will bind to ":http" (port 80) and ignore the configured endpoint.
		if err := r.server.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			r.settings.Logger.Error("Server error", zap.Error(err))
		}
	}()

	return nil
}

func (r *lumigoReceiver) Shutdown(ctx context.Context) error {
	if r.server != nil {
		return r.server.Shutdown(ctx)
	}
	return nil
}

func (r *lumigoReceiver) handleTraces(w http.ResponseWriter, req *http.Request) {
	ctx := req.Context()

	// Only accept POST requests
	if req.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read request body
	body, err := io.ReadAll(req.Body)
	if err != nil {
		r.settings.Logger.Error("Failed to read request body", zap.Error(err))
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	// Log the received data size
	r.settings.Logger.Debug("Received Lumigo spans",
		zap.Int("bytes", len(body)),
		zap.String("content_type", req.Header.Get("Content-Type")))

	// Unmarshal Lumigo spans
	lumigoSpans, err := unmarshalLumigoSpans(body)
	if err != nil {
		r.settings.Logger.Error("Failed to unmarshal Lumigo spans",
			zap.Error(err),
			zap.String("body_preview", getBodyPreview(body)))
		http.Error(w, fmt.Sprintf("Failed to parse Lumigo spans: %v", err), http.StatusBadRequest)
		return
	}

	r.settings.Logger.Debug("Unmarshaled Lumigo spans", zap.Int("count", len(lumigoSpans)))

	// Transform to OTLP
	traces, err := transformLumigoToOTLP(lumigoSpans)
	if err != nil {
		r.settings.Logger.Error("Failed to transform Lumigo spans to OTLP",
			zap.Error(err),
			zap.Int("span_count", len(lumigoSpans)))
		http.Error(w, fmt.Sprintf("Failed to transform spans: %v", err), http.StatusInternalServerError)
		return
	}

	r.settings.Logger.Debug("Transformed to OTLP",
		zap.Int("resource_spans", traces.ResourceSpans().Len()))

	// Send to consumer
	if err := r.consumer.ConsumeTraces(ctx, traces); err != nil {
		r.settings.Logger.Error("Failed to consume traces",
			zap.Error(err),
			zap.Int("span_count", len(lumigoSpans)))
		http.Error(w, "Failed to process traces", http.StatusInternalServerError)
		return
	}

	r.settings.Logger.Debug("Successfully processed Lumigo spans", zap.Int("count", len(lumigoSpans)))

	// Return success
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"ok"}`))
}

func (r *lumigoReceiver) handleHealth(w http.ResponseWriter, req *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`{"status":"healthy"}`))
}

// getBodyPreview returns the first 200 characters of the body for logging
func getBodyPreview(body []byte) string {
	if len(body) > 200 {
		return string(body[:200]) + "..."
	}
	return string(body)
}
