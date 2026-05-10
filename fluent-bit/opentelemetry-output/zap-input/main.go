// Sends a few test log records via Uber's zap logger. The output target
// is selectable via the -output flag:
//
//   -output otlp    (default) Bridge zap to the OpenTelemetry Go SDK's
//                   OTLP/HTTP log exporter, sending to Fluent Bit's
//                   opentelemetry input on 127.0.0.1:4318. Run
//                   ./start-fluent-bit in another shell first.
//
//   -output stdout  Use a plain zap production logger that writes JSON
//                   log lines directly to stdout (no OTLP, no network).
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
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
	output := flag.String("output", "otlp", "log output target: 'otlp' or 'stdout'")
	flag.Parse()

	logger, shutdown, err := buildLogger(*output)
	if err != nil {
		log.Fatalf("build logger (%s): %v", *output, err)
	}
	defer func() {
		_ = logger.Sync()
		if shutdown != nil {
			shutdown()
		}
	}()

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

	log.Printf("emitted 3 zap log records via %s; flushing...", *output)
}

func buildLogger(output string) (*zap.Logger, func(), error) {
	switch output {
	case "stdout":
		cfg := zap.NewProductionConfig()
		cfg.OutputPaths = []string{"stdout"}
		cfg.ErrorOutputPaths = []string{"stderr"}
		logger, err := cfg.Build()
		if err != nil {
			return nil, nil, fmt.Errorf("zap stdout config: %w", err)
		}
		return logger, nil, nil

	case "otlp":
		ctx := context.Background()

		exp, err := otlploghttp.New(ctx,
			otlploghttp.WithEndpoint(otlpEndpoint),
			otlploghttp.WithInsecure(),
		)
		if err != nil {
			return nil, nil, fmt.Errorf("create OTLP log exporter: %w", err)
		}

		res, err := resource.Merge(
			resource.Default(),
			resource.NewWithAttributes(semconv.SchemaURL,
				semconv.ServiceName(serviceName),
			),
		)
		if err != nil {
			return nil, nil, fmt.Errorf("build resource: %w", err)
		}

		provider := sdklog.NewLoggerProvider(
			sdklog.WithResource(res),
			sdklog.WithProcessor(sdklog.NewBatchProcessor(exp)),
		)
		global.SetLoggerProvider(provider)

		shutdown := func() {
			shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if err := provider.Shutdown(shutdownCtx); err != nil {
				log.Printf("provider shutdown: %v", err)
			}
		}

		return zap.New(otelzap.NewCore(serviceName)), shutdown, nil

	default:
		fmt.Fprintf(os.Stderr, "unknown -output %q (expected 'otlp' or 'stdout')\n", output)
		flag.Usage()
		os.Exit(2)
		return nil, nil, nil
	}
}
