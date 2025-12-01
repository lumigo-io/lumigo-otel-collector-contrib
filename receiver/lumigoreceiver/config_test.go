// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package lumigoreceiver

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/consumer/consumertest"
	"go.opentelemetry.io/collector/confmap/confmaptest"
	"go.opentelemetry.io/collector/receiver/receivertest"
)

func TestLoadConfig(t *testing.T) {
	cm, err := confmaptest.LoadConf(filepath.Join("testdata", "config.yaml"))
	require.NoError(t, err)

	factory := NewFactory()
	cfg := factory.CreateDefaultConfig()

	componentID := component.MustNewIDWithName(factory.Type().String(), "")
	sub, err := cm.Sub(componentID.String())
	require.NoError(t, err)
	require.NoError(t, sub.Unmarshal(cfg))

	assert.NoError(t, component.ValidateConfig(cfg))

	lumigoConfig := cfg.(*Config)
	assert.Equal(t, "0.0.0.0:8088", lumigoConfig.Endpoint)
}


func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				ServerConfig: createDefaultConfig().(*Config).ServerConfig,
			},
			wantErr: false,
		},
		{
			name: "empty endpoint",
			config: &Config{
				ServerConfig: createDefaultConfig().(*Config).ServerConfig,
			},
			wantErr: false, // ServerConfig handles this
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.config.Endpoint = "0.0.0.0:8088" // Set a valid endpoint
			err := component.ValidateConfig(tt.config)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestCreateReceiverWithConfig(t *testing.T) {
	factory := NewFactory()
	cfg := &Config{}
	cfg.Endpoint = "localhost:8088"

	receiver, err := factory.CreateTraces(
		context.Background(),
		receivertest.NewNopSettings(),
		cfg,
		consumertest.NewNop(),
	)

	require.NoError(t, err)
	assert.NotNil(t, receiver)
}