package main

import (
	"context"
	"os"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "isucondition"

type traceSpan struct {
	span trace.Span
}

func (s traceSpan) End() {
	if s.span != nil {
		s.span.End()
	}
}

func (s traceSpan) Bool(key string, v bool) {
	if s.span != nil {
		s.span.SetAttributes(attribute.Bool(key, v))
	}
}

func (s traceSpan) Int(key string, v int) {
	if s.span != nil {
		s.span.SetAttributes(attribute.Int(key, v))
	}
}

func (s traceSpan) Int64(key string, v int64) {
	if s.span != nil {
		s.span.SetAttributes(attribute.Int64(key, v))
	}
}

func (s traceSpan) RecordError(err error) {
	if s.span != nil && err != nil {
		s.span.RecordError(err)
		s.span.SetStatus(codes.Error, err.Error())
	}
}

func initTracing() (func(context.Context) error, error) {
	if getEnv("ENABLE_TRACING", "0") != "1" {
		return func(context.Context) error { return nil }, nil
	}
	ctx := context.Background()
	endpoint := getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "127.0.0.1:4318")
	exp, err := otlptracehttp.New(ctx,
		otlptracehttp.WithEndpoint(endpoint),
		otlptracehttp.WithInsecure(),
	)
	if err != nil {
		return nil, err
	}
	host, _ := os.Hostname()
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName("isucondition"),
			semconv.HostName(host),
		),
	)
	if err != nil {
		return nil, err
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exp, sdktrace.WithBatchTimeout(time.Second)),
		sdktrace.WithSampler(sdktrace.AlwaysSample()),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)
	return tp.Shutdown, nil
}

func startTrace(ctx context.Context, name string) (context.Context, traceSpan) {
	ctx, span := otel.Tracer(tracerName).Start(ctx, name)
	return ctx, traceSpan{span: span}
}

func traceCall(ctx context.Context, name string, fn func(context.Context) error) error {
	ctx, span := startTrace(ctx, name)
	defer span.End()
	if err := fn(ctx); err != nil {
		span.RecordError(err)
		return err
	}
	return nil
}
