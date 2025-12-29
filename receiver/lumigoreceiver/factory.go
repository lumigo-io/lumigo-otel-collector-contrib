// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

package lumigoreceiver // import "github.com/lumigo-io/lumigo-otel-collector-contrib/receiver/lumigoreceiver"

import (
	"context"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/consumer"
	"go.opentelemetry.io/collector/receiver"
)

// TypeStr is the value of "type" key in configuration.
const TypeStr = "lumigo"

var receiverType = component.MustNewType(TypeStr)

// NewFactory creates a factory for Lumigo receiver.
func NewFactory() receiver.Factory {
	return receiver.NewFactory(
		receiverType,
		createDefaultConfig,
		receiver.WithTraces(createTracesReceiver, component.StabilityLevelAlpha))
}

func createDefaultConfig() component.Config {
	return &Config{
		ServerConfig: confighttp.ServerConfig{
			Endpoint: "0.0.0.0:8088",
		},
	}
}

func createTracesReceiver(
	_ context.Context,
	set receiver.Settings,
	cfg component.Config,
	consumer consumer.Traces,
) (receiver.Traces, error) {
	rCfg := cfg.(*Config)
	return newLumigoReceiver(rCfg, consumer, set)
}
