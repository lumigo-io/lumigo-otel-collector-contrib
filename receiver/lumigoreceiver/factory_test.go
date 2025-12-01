// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package lumigoreceiver

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/receiver/receivertest"
)

func TestDefaultConfig(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig()
	assert.NotNil(t, cfg, "failed to create default config")
	assert.NoError(t, component.ValidateConfig(cfg))

	lumigoConfig := cfg.(*Config)
	assert.Equal(t, "0.0.0.0:8088", lumigoConfig.Endpoint)
}

func TestCreateReceiver(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig()

	receiver, err := factory.CreateTraces(
		context.Background(),
		receivertest.NewNopSettings(),
		cfg,
		consumertest.NewNop(),
	)

	require.NoError(t, err)
	assert.NotNil(t, receiver)
}

func TestCreateReceiverWithNilConsumer(t *testing.T) {
	factory := NewFactory()
	cfg := factory.CreateDefaultConfig()

	_, err := factory.CreateTraces(
		context.Background(),
		receivertest.NewNopSettings(),
		cfg,
		nil,
	)

	assert.Error(t, err)
}

func TestFactoryType(t *testing.T) {
	factory := NewFactory()
	assert.Equal(t, "lumigo", factory.Type().String())
}
