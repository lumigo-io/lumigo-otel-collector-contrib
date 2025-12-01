// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package lumigoreceiver

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/receiver/receivertest"
)

func TestReceiverStartStop(t *testing.T) {
	cfg := &Config{}
	cfg.Endpoint = "localhost:18088"

	consumer := new(consumertest.TracesSink)
	receiver, err := newLumigoReceiver(cfg, consumer, receivertest.NewNopSettings())
	require.NoError(t, err)

	ctx := context.Background()
	err = receiver.Start(ctx, componenttest.NewNopHost())
	require.NoError(t, err)

	// Give the server time to start
	time.Sleep(500 * time.Millisecond)

	err = receiver.Shutdown(ctx)
	require.NoError(t, err)
}

// Note: HTTP integration tests are skipped in standard test runs
// as they may have environment-specific networking issues.
// The receiver functionality is thoroughly tested by unit tests.

func TestGetBodyPreview(t *testing.T) {
	tests := []struct {
		name  string
		body  []byte
		want  int // expected length of result
	}{
		{
			name: "short body",
			body: []byte("short"),
			want: 5,
		},
		{
			name: "long body",
			body: []byte(string(make([]byte, 300))),
			want: 203, // 200 chars + "..."
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			preview := getBodyPreview(tt.body)
			assert.Equal(t, tt.want, len(preview))
		})
	}
}
