package main

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/XSAM/otelsql"
	"github.com/labstack/echo/v4"
	"github.com/valkey-io/valkey-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

const (
	tracerScopeIsucondition = "isucondition"
	serviceIsucondition     = "isucondition"
	serviceMySQL            = "mysql"
	serviceValkey           = "valkey"
	serviceJiamock          = "jiamock"
)

var (
	tracingActive        bool
	mysqlTracerProvider  trace.TracerProvider
	valkeyTracerProvider trace.TracerProvider
	jiaTracerProvider    trace.TracerProvider
)

func tracingEnabled() bool {
	return tracingActive
}

func initTracing() (func(context.Context) error, error) {
	if getEnv("ENABLE_TRACING", "1") == "0" {
		tracingActive = false
		return func(context.Context) error { return nil }, nil
	}
	ctx := context.Background()
	exp, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, err
	}
	host, _ := os.Hostname()

	appTP, err := newServiceTracerProvider(ctx, exp, serviceIsucondition, host)
	if err != nil {
		return nil, err
	}
	mysqlTP, err := newServiceTracerProvider(ctx, exp, serviceMySQL, host)
	if err != nil {
		return nil, err
	}
	valkeyTP, err := newServiceTracerProvider(ctx, exp, serviceValkey, host)
	if err != nil {
		return nil, err
	}
	jiaTP, err := newServiceTracerProvider(ctx, exp, serviceJiamock, host)
	if err != nil {
		return nil, err
	}

	otel.SetTracerProvider(appTP)
	mysqlTracerProvider = mysqlTP
	valkeyTracerProvider = valkeyTP
	jiaTracerProvider = jiaTP
	tracingActive = true

	return func(ctx context.Context) error {
		var lastErr error
		for _, shutdown := range []func(context.Context) error{
			appTP.Shutdown, mysqlTP.Shutdown, valkeyTP.Shutdown, jiaTP.Shutdown,
		} {
			if err := shutdown(ctx); err != nil {
				lastErr = err
			}
		}
		return lastErr
	}, nil
}

func newServiceTracerProvider(ctx context.Context, exp sdktrace.SpanExporter, serviceName, host string) (*sdktrace.TracerProvider, error) {
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
			semconv.HostName(host),
		),
	)
	if err != nil {
		return nil, err
	}
	return sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp, sdktrace.WithBatchTimeout(time.Second)),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithResource(res),
	), nil
}

func echoTraceMiddleware() echo.MiddlewareFunc {
	if !tracingEnabled() {
		return func(next echo.HandlerFunc) echo.HandlerFunc { return next }
	}
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			req := c.Request()
			route := c.Path()
			ctx, span := otel.Tracer(tracerScopeIsucondition).Start(req.Context(), req.Method+" "+route)
			defer span.End()
			span.SetAttributes(
				semconv.HTTPRequestMethodKey.String(req.Method),
				semconv.HTTPRouteKey.String(route),
			)
			c.SetRequest(req.WithContext(ctx))
			err := next(c)
			if err != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}
			span.SetAttributes(semconv.HTTPResponseStatusCodeKey.Int(c.Response().Status))
			return err
		}
	}
}

func openTracedSQL(dsn string) (*sql.DB, error) {
	return otelsql.Open("mysql", dsn,
		otelsql.WithTracerProvider(mysqlTracerProvider),
		otelsql.WithAttributes(semconv.DBSystemMySQL),
	)
}

type tracingValkeyClient struct {
	valkey.Client
}

func wrapValkeyClient(client valkey.Client) valkey.Client {
	if !tracingEnabled() {
		return client
	}
	return &tracingValkeyClient{Client: client}
}

func (c *tracingValkeyClient) Do(ctx context.Context, cmd valkey.Completed) valkey.ValkeyResult {
	ctx, span := valkeyTracerProvider.Tracer("valkey-go").Start(ctx, "valkey.do")
	defer span.End()
	res := c.Client.Do(ctx, cmd)
	if err := res.Error(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	return res
}

func (c *tracingValkeyClient) DoMulti(ctx context.Context, multi ...valkey.Completed) []valkey.ValkeyResult {
	ctx, span := valkeyTracerProvider.Tracer("valkey-go").Start(ctx, "valkey.do_multi")
	span.SetAttributes(attribute.Int("valkey.command_count", len(multi)))
	defer span.End()
	resps := c.Client.DoMulti(ctx, multi...)
	for _, res := range resps {
		if err := res.Error(); err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			break
		}
	}
	return resps
}

func doJIARequest(ctx context.Context, req *http.Request) (*http.Response, error) {
	if !tracingEnabled() {
		return http.DefaultClient.Do(req.WithContext(ctx))
	}
	ctx, span := jiaTracerProvider.Tracer("http").Start(ctx, req.Method+" "+req.URL.Path, trace.WithSpanKind(trace.SpanKindClient))
	defer span.End()
	span.SetAttributes(
		semconv.HTTPRequestMethodKey.String(req.Method),
		semconv.URLPath(req.URL.Path),
		semconv.ServerAddress(req.URL.Hostname()),
	)
	res, err := http.DefaultClient.Do(req.WithContext(ctx))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return res, err
	}
	span.SetAttributes(semconv.HTTPResponseStatusCodeKey.Int(res.StatusCode))
	if res.StatusCode >= 400 {
		span.SetStatus(codes.Error, res.Status)
	}
	return res, nil
}

func traceLock(ctx context.Context, name string, mu *sync.Mutex) (context.Context, func()) {
	if !tracingEnabled() {
		mu.Lock()
		return ctx, mu.Unlock
	}
	waitStart := time.Now()
	mu.Lock()
	wait := time.Since(waitStart)
	ctx, span := otel.Tracer(tracerScopeIsucondition).Start(ctx, "lock "+name)
	span.SetAttributes(attribute.Int64("lock.wait_ms", wait.Milliseconds()))
	return ctx, func() {
		span.End()
		mu.Unlock()
	}
}

func spanFromContext(ctx context.Context) trace.Span {
	return trace.SpanFromContext(ctx)
}

func setSpanBool(span trace.Span, key string, v bool) {
	if span != nil && span.IsRecording() {
		span.SetAttributes(attribute.Bool(key, v))
	}
}

func setSpanInt(span trace.Span, key string, v int) {
	if span != nil && span.IsRecording() {
		span.SetAttributes(attribute.Int(key, v))
	}
}

func recordSpanError(span trace.Span, err error) {
	if span != nil && span.IsRecording() && err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}
