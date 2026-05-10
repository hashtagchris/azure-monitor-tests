// aks-otel-zap is a minimal HTTP service used to validate the AKS Application
// Monitoring (preview) OTLP pipeline. It logs with Uber zap, bridges those
// logs into the OpenTelemetry logs SDK via contrib/bridges/otelzap, and
// exports them over OTLP/HTTP. The exporter is configured purely from
// environment variables (OTEL_EXPORTER_OTLP_ENDPOINT, etc.) which the
// Application Monitoring addon injects when the pod's namespace is onboarded
// in autoconfiguration mode.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync/atomic"
	"syscall"
	"time"

	"go.opentelemetry.io/contrib/bridges/otelzap"
	"go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp"
	"go.opentelemetry.io/otel/log/global"
	"go.opentelemetry.io/otel/sdk/log"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const (
	serviceName    = "aks-otel-zap"
	serviceVersion = "0.1.0"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	shutdown, err := initOTel(ctx)
	if err != nil {
		fmt.Fprintf(os.Stderr, "init otel: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := shutdown(shutdownCtx); err != nil {
			fmt.Fprintf(os.Stderr, "shutdown otel: %v\n", err)
		}
	}()

	logger := newLogger()
	defer func() { _ = logger.Sync() }()

	logger.Info("starting aks-otel-zap",
		zap.String("service.version", serviceVersion),
		zap.String("k8s.pod.name", os.Getenv("POD_NAME")),
		zap.String("k8s.namespace.name", os.Getenv("POD_NAMESPACE")),
		zap.String("k8s.node.name", os.Getenv("NODE_NAME")),
	)

	go heartbeat(ctx, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("/log", logHandler(logger))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = fmt.Fprintln(w, "ok")
	})

	srv := &http.Server{
		Addr:              ":8080",
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	serverErr := make(chan error, 1)
	go func() {
		logger.Info("http server listening", zap.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serverErr <- err
		}
		close(serverErr)
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
	case err := <-serverErr:
		if err != nil {
			logger.Error("http server failed", zap.Error(err))
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}

// initOTel configures the OpenTelemetry global LoggerProvider with an
// OTLP/HTTP exporter. Endpoint, headers, and protocol come from the
// standard OTEL_EXPORTER_OTLP_* env vars injected by the AKS Application
// Monitoring addon.
func initOTel(ctx context.Context) (func(context.Context) error, error) {
	res, err := resource.New(ctx,
		resource.WithFromEnv(),
		resource.WithProcess(),
		resource.WithHost(),
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.ServiceVersion(serviceVersion),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("build resource: %w", err)
	}

	exp, err := otlploghttp.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("build otlp log exporter: %w", err)
	}

	provider := log.NewLoggerProvider(
		log.WithResource(res),
		log.WithProcessor(log.NewBatchProcessor(exp)),
	)
	global.SetLoggerProvider(provider)

	return provider.Shutdown, nil
}

// newLogger wires a zap logger that writes structured JSON to stdout AND
// bridges every entry into the global OTel LoggerProvider via otelzap.
// This way pod logs remain visible to `kubectl logs` while also flowing to
// Azure Monitor.
func newLogger() *zap.Logger {
	encCfg := zap.NewProductionEncoderConfig()
	encCfg.TimeKey = "timestamp"
	encCfg.EncodeTime = zapcore.ISO8601TimeEncoder

	stdoutCore := zapcore.NewCore(
		zapcore.NewJSONEncoder(encCfg),
		zapcore.Lock(os.Stdout),
		zap.InfoLevel,
	)

	otelCore := otelzap.NewCore(
		serviceName,
		otelzap.WithLoggerProvider(global.GetLoggerProvider()),
	)

	return zap.New(
		zapcore.NewTee(stdoutCore, otelCore),
		zap.AddCaller(),
	)
}

// logHandler emits a single zap log line for each request.
// Usage:
//
//	GET /log?level=info|warn|error&msg=hello&foo=bar
//
// Any additional query params are attached as structured fields.
func logHandler(logger *zap.Logger) http.HandlerFunc {
	var counter atomic.Uint64
	return func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		level := strings.ToLower(q.Get("level"))
		msg := q.Get("msg")
		if msg == "" {
			msg = "log endpoint hit"
		}

		fields := []zap.Field{
			zap.Uint64("request.seq", counter.Add(1)),
			zap.String("http.method", r.Method),
			zap.String("http.target", r.URL.Path),
			zap.String("net.peer.addr", r.RemoteAddr),
			zap.String("key1", "value1"),
			zap.String("key2", "value2"),
		}
		for k, vs := range q {
			if k == "level" || k == "msg" {
				continue
			}
			fields = append(fields, zap.String("query."+k, strings.Join(vs, ",")))
		}

		switch level {
		case "warn", "warning":
			logger.Warn(msg, fields...)
		case "error", "err":
			logger.Error(msg, fields...)
		case "debug":
			logger.Debug(msg, fields...)
		default:
			logger.Info(msg, fields...)
		}

		_, _ = fmt.Fprintf(w, "logged level=%q msg=%q\n", level, msg)
	}
}

// heartbeat emits a single INFO log every 15 minutes (configurable via
// HEARTBEAT_INTERVAL, e.g. "1m" for local testing). Useful for verifying
// the end-to-end pipeline without external traffic.
func heartbeat(ctx context.Context, logger *zap.Logger) {
	interval := 15 * time.Minute
	if v := os.Getenv("HEARTBEAT_INTERVAL"); v != "" {
		if parsed, err := time.ParseDuration(v); err == nil {
			interval = parsed
		}
	}

	start := time.Now()
	var seq uint64
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			seq++
			logger.Info("heartbeat",
				zap.Uint64("heartbeat.seq", seq),
				zap.Duration("heartbeat.interval", interval),
				zap.Duration("process.uptime", time.Since(start).Round(time.Second)),
			)
		}
	}
}
