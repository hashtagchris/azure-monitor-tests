// Sends a few test log records via Uber's zap logger. The output target
// is selectable via the -output flag:
//
//	-output otlp    (default) Bridge zap to the OpenTelemetry Go SDK's
//	                OTLP/HTTP log exporter, sending to Fluent Bit's
//	                opentelemetry input on 127.0.0.1:4318. Run
//	                ./start-fluent-bit in another shell first.
//
//	-output stdout  Use a plain zap production logger that writes JSON
//	                log lines directly to stdout (no OTLP, no network).
//
//	-output logfmt  Write logfmt-formatted lines to stdout, using the
//	                github.com/jsternberg/zap-logfmt encoder.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	logfmt "github.com/jsternberg/zap-logfmt"
	"go.opentelemetry.io/contrib/bridges/otelzap"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/log/global"
	sdklog "go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.40.0"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	otlpEndpoint = "127.0.0.1:4318"
	serviceName  = "zap-input-test"
	scopeVersion = "0.1.0"
	traceIDHex   = "5b8efff798038103d269b633813fc60c"
	spanIDHex    = "eee19b7ec3c1b173"
)

func main() {
	output := flag.String("output", "otlp", "log output target: 'otlp', 'stdout', or 'logfmt'")
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

	// Attach OTel attributes to every record, regardless of the output target.
	// Alternative: See the resource attributes in buildLogger() for attributes that are attached to every record only when using the OTLP output.
	logger = logger.With(
		zap.String(string(semconv.ClientAddressKey), "24.211.200.42"),
		zap.String(string(semconv.UserAgentOriginalKey), "test-zap-input/1.0"),
		zap.String(string(semconv.K8SPodNameKey), "two-peas"),
	)
	logger = logger.With(traceFields(*output)...)

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

// traceCtx is a Go context carrying a fixed test SpanContext. The otelzap
// bridge picks up a context.Context attached via zap.Reflect and uses it
// when emitting OTLP log records, so the embedded TraceID/SpanID end up
// on the OTLP log record itself (not as attributes).
var traceCtx = func() context.Context {
	tid, err := trace.TraceIDFromHex(traceIDHex)
	if err != nil {
		log.Fatalf("parse trace id: %v", err)
	}
	sid, err := trace.SpanIDFromHex(spanIDHex)
	if err != nil {
		log.Fatalf("parse span id: %v", err)
	}
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID:    tid,
		SpanID:     sid,
		TraceFlags: trace.FlagsSampled,
		Remote:     true,
	})
	return trace.ContextWithSpanContext(context.Background(), sc)
}()

// traceFields returns the per-record fields used to surface the test
// TraceID/SpanID. For OTLP they ride along inside a context.Context that
// the otelzap bridge unwraps, lifting them to the OTLP record's
// TraceId/SpanId fields. For stdout/logfmt we emit them as ordinary
// string fields so they're visible in the output.
func traceFields(output string) []zap.Field {
	if output == "otlp" {
		return []zap.Field{zap.Reflect("ctx", traceCtx)}
	}
	return []zap.Field{
		zap.String("trace_id", traceIDHex),
		zap.String("span_id", spanIDHex),
	}
}

func buildLogger(output string) (*zap.Logger, func(), error) {	switch output {
	case "stdout":
		cfg := zap.NewProductionConfig()
		cfg.OutputPaths = []string{"stdout"}
		cfg.ErrorOutputPaths = []string{"stderr"}
		logger, err := cfg.Build()
		if err != nil {
			return nil, nil, fmt.Errorf("zap stdout config: %w", err)
		}
		return logger, nil, nil

	case "logfmt":
		encCfg := zap.NewProductionEncoderConfig()
		encCfg.EncodeTime = zapcore.ISO8601TimeEncoder
		encoder := logfmt.NewEncoder(encCfg)
		core := zapcore.NewCore(encoder, zapcore.Lock(os.Stdout), zapcore.DebugLevel)
		return zap.New(core, zap.AddCaller()), nil, nil

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
				// service.name
				semconv.ServiceName(serviceName),

				// service.version
				semconv.ServiceVersion("0.0.1"),

				// service.namespace
				semconv.ServiceNamespace("test-namespace"),

				// deployment.environment.name
				semconv.DeploymentEnvironmentName("test"),
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

		return zap.New(otelzap.NewCore(serviceName,
			otelzap.WithVersion(scopeVersion),
		)), shutdown, nil

	default:
		fmt.Fprintf(os.Stderr, "unknown -output %q (expected 'otlp', 'stdout', or 'logfmt')\n", output)
		flag.Usage()
		os.Exit(2)
		return nil, nil, nil
	}
}
