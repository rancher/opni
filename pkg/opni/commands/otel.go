package commands

import (
	prometheusremotewriteexporter "github.com/open-telemetry/opentelemetry-collector-contrib/exporter/prometheusremotewriteexporter"
	"github.com/spf13/cobra"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/exporter"
	otlpexporter "go.opentelemetry.io/collector/exporter/otlpexporter"
	"go.opentelemetry.io/collector/otelcol"
	"go.opentelemetry.io/collector/receiver"
	otlpreceiver "go.opentelemetry.io/collector/receiver/otlpreceiver"
)

func BuildOtelCmd() *cobra.Command {
	factories := otelcol.Factories{}

	var err error
	factories.Receivers, err = receiver.MakeFactoryMap(
		otlpreceiver.NewFactory(),
	)
	if err != nil {
		panic(err)
	}

	factories.Exporters, err = exporter.MakeFactoryMap(
		otlpexporter.NewFactory(),
		prometheusremotewriteexporter.NewFactory(),
	)
	if err != nil {
		panic(err)
	}

	return otelcol.NewCommand(otelcol.CollectorSettings{
		BuildInfo: component.BuildInfo{
			Command:     "otel",
			Description: "Custom OpenTelemetry Collector distribution",
			Version:     "0.73.0",
		},
		Factories: factories,
	})
}

func init() {
	AddCommandsToGroup(OpniComponents, BuildOtelCmd())
}
