// Sends a few test log records via Uber's zap logger, bridged through the
// OpenTelemetry Go SDK's OTLP/HTTP log exporter, to Fluent Bit's
// opentelemetry input. Run `./start-fluent-bit` in another shell first.
package main

import (
	"context"
	"log"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelzap"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.uber.org/zap"
)

const (
	otlpEndpoint = "127.0.0.1:4318"
	serviceName  = "zap-input-test"
)

func main() {
	ctx := context.Background()

	exp, err := otlploghttp.New(ctx,
		otlploghttp.WithEndpoint(otlpEndpoint),
		otlploghttp.WithInsecure(),
	)
	if err != nil {
		log.Fatalf("create OTLP log exporter: %v", err)
	}

	res, err := resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(semconv.SchemaURL,
			semconv.ServiceName(serviceName),
		),
	)
	if err != nil {
		log.Fatalf("build resource: %v", err)
	}

	provider := sdklog.NewLoggerProvider(
		sdklog.WithResource(res),
		sdklog.WithProcessor(sdklog.NewBatchProcessor(exp)),
	)
	global.SetLoggerProvider(provider)
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := provider.Shutdown(shutdownCtx); err != nil {
			log.Printf("provider shutdown: %v", err)
		}
	}()

	logger := zap.New(otelzap.NewCore(serviceName))
	defer func() { _ = logger.Sync() }()

	logger.Info("hello from zap via OTLP",
		zap.String("key1", "value1"),
		zap.String("key2", "value2"),
	)
	logger.Warn("zap warning sample",
		zap.Int("attempt", 3),
		zap.String("component", "demo"),
	)
	logger.Error("zap error sample",
		zap.String("error_kind", "synthetic"),
	)

	log.Println("emitted 3 zap log records; flushing...")
}
