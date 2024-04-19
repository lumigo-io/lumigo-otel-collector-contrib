package main

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componenttest"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/extension/extensiontest"

	"github.com/lumigo-io/lumigo-otel-collector-contrib/extension/lumigoauthextension"
)

func TestDefaultExtensions(t *testing.T) {
	allFactories, err := components()
	require.NoError(t, err)

	extFactories := allFactories.Extensions

	tests := []struct {
		getConfigFn   getExtensionConfigFn
		extension     component.Type
		skipLifecycle bool
	}{
		{
			extension: component.MustNewType("lumigoauthextension"),
			getConfigFn: func() component.Config {
				cfg := extFactories[component.MustNewType("lumigoauthextension")].CreateDefaultConfig().(*lumigoauthextension.Config)
				cfg.Token = "test-token"
				return cfg
			},
		},
	}
	extensionCount := 0
	expectedExtensions := map[component.Type]struct{}{}
	for k := range extFactories {
		expectedExtensions[k] = struct{}{}
	}
	for _, tt := range tests {
		_, ok := extFactories[tt.extension]
		if !ok {
			// not part of the distro, skipping.
			continue
		}
		delete(expectedExtensions, tt.extension)
		extensionCount++
		t.Run(string(tt.extension.String()), func(t *testing.T) {
			factory := extFactories[tt.extension]
			assert.Equal(t, tt.extension, factory.Type())

			t.Run("shutdown", func(t *testing.T) {
				verifyExtensionShutdown(t, factory, tt.getConfigFn)
			})
			t.Run("lifecycle", func(t *testing.T) {
				if tt.skipLifecycle {
					t.SkipNow()
				}
				verifyExtensionLifecycle(t, factory, tt.getConfigFn)
			})

		})
	}
	assert.Len(t, extFactories, extensionCount, "All extensions must be added to the lifecycle tests", expectedExtensions)
}

// getExtensionConfigFn is used customize the configuration passed to the verification.
// This is used to change ports or provide values required but not provided by the
// default configuration.
type getExtensionConfigFn func() component.Config

// verifyExtensionLifecycle is used to test if an extension type can handle the typical
// lifecycle of a component. The getConfigFn parameter only need to be specified if
// the test can't be done with the default configuration for the component.
func verifyExtensionLifecycle(t *testing.T, factory extension.Factory, getConfigFn getExtensionConfigFn) {
	ctx := context.Background()
	host := componenttest.NewNopHost()
	extCreateSet := extensiontest.NewNopCreateSettings()
	extCreateSet.ReportStatus = func(event *component.StatusEvent) {
		require.NoError(t, event.Err())
	}

	if getConfigFn == nil {
		getConfigFn = factory.CreateDefaultConfig
	}

	firstExt, err := factory.CreateExtension(ctx, extCreateSet, getConfigFn())
	require.NoError(t, err)
	require.NoError(t, firstExt.Start(ctx, host))
	require.NoError(t, firstExt.Shutdown(ctx))

	secondExt, err := factory.CreateExtension(ctx, extCreateSet, getConfigFn())
	require.NoError(t, err)
	require.NoError(t, secondExt.Start(ctx, host))
	require.NoError(t, secondExt.Shutdown(ctx))
}

// verifyExtensionShutdown is used to test if an extension type can be shutdown without being started first.
func verifyExtensionShutdown(tb testing.TB, factory extension.Factory, getConfigFn getExtensionConfigFn) {
	ctx := context.Background()
	extCreateSet := extensiontest.NewNopCreateSettings()

	if getConfigFn == nil {
		getConfigFn = factory.CreateDefaultConfig
	}

	e, err := factory.CreateExtension(ctx, extCreateSet, getConfigFn())
	if errors.Is(err, component.ErrDataTypeIsNotSupported) {
		return
	}
	if e == nil {
		return
	}

	assert.NotPanics(tb, func() {
		assert.NoError(tb, e.Shutdown(ctx))
	})
}
