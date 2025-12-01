// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package lumigoreceiver // import "github.com/lumigo-io/lumigo-otel-collector-contrib/receiver/lumigoreceiver"

import (
	"errors"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
)

// Config defines configuration for the Lumigo receiver.
type Config struct {
	// ServerConfig contains HTTP server settings.
	// The default endpoint is 0.0.0.0:8088.
	confighttp.ServerConfig `mapstructure:",squash"`
}

// Validate checks if the receiver configuration is valid.
func (cfg *Config) Validate() error {
	if cfg.Endpoint == "" {
		return errors.New("endpoint must be specified")
	}
	return nil
}

var _ component.Config = (*Config)(nil)
