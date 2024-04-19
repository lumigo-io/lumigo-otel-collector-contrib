package components // import "github.com/lumigo-io/lumigo-otel-collector-contrib/internal/components"

import (
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/processor"

	"github.com/lumigo-io/lumigo-otel-collector-contrib/extension/lumigoauthextension"
	"github.com/lumigo-io/lumigo-otel-collector-contrib/processor/k8seventsenricherprocessor"
	"github.com/lumigo-io/lumigo-otel-collector-contrib/processor/redactionbykeyprocessor"
)

func Components() (otelcol.Factories, error) {
	var err error
	factories := otelcol.Factories{}
	extensions := []extension.Factory{
		lumigoauthextension.NewFactory(),
	}
	factories.Extensions, err = extension.MakeFactoryMap(extensions...)
	if err != nil {
		return otelcol.Factories{}, err
	}

	processors := []processor.Factory{
		k8seventsenricherprocessor.NewFactory(),
		redactionbykeyprocessor.NewFactory(),
	}
	factories.Processors, err = processor.MakeFactoryMap(processors...)
	if err != nil {
		return otelcol.Factories{}, err
	}

	return factories, nil
}
