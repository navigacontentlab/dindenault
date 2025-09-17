// Package telemetry provides monitoring and observability tools for dindenault.
package telemetry

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"connectrpc.com/connect"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch"
	"github.com/aws/aws-sdk-go-v2/service/cloudwatch/types"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.24.0"
	"go.opentelemetry.io/otel/trace"
)

// Constants for telemetry.
const (
	// UnknownValue is used when the real value cannot be determined.
	UnknownValue = "unknown"

	// Version of the telemetry package.
	Version = "0.1.0"
)

// Options configures OpenTelemetry and CloudWatch metrics.
type Options struct {
	// MetricNamespace is the CloudWatch namespace for metrics
	MetricNamespace string

	// OrganizationFn extracts organization from context
	OrganizationFn func(ctx context.Context) string

	// AWSConfig is the AWS configuration to use for CloudWatch
	AWSConfig aws.Config

	// MetricAttributes are additional attributes to add to all metrics
	MetricAttributes []attribute.KeyValue
}

// DefaultOrganizationFunction returns a function that always returns "unknown".
// This is a safe default for when navigaid is not available.
// To use navigaid organization extraction, import the navigaid module and create your own function.
func DefaultOrganizationFunction() func(ctx context.Context) string {
	return func(ctx context.Context) string {
		return UnknownValue
	}
}

// Initialize initializes OpenTelemetry with CloudWatch metrics and tracing export.
func Initialize(ctx context.Context, serviceName string, opts *Options) (func(context.Context) error, error) {
	// Build resource with service metadata
	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(serviceName),
		),
		resource.WithAttributes(opts.MetricAttributes...),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create OTLP trace exporter
	traceExporter, err := otlptracegrpc.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP trace exporter: %w", err)
	}

	// Create TracerProvider
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithResource(res),
		sdktrace.WithBatcher(traceExporter),
	)

	// Set the global TracerProvider
	otel.SetTracerProvider(tp)

	// Set the global TextMapPropagator to handle context propagation
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	// Create OTLP metrics exporter
	metricsExporter, err := otlpmetricgrpc.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP metrics exporter: %w", err)
	}

	// Create MeterProvider with the exporter
	mp := sdkmetric.NewMeterProvider(
		sdkmetric.WithResource(res),
		sdkmetric.WithReader(
			sdkmetric.NewPeriodicReader(
				metricsExporter,
				sdkmetric.WithInterval(60*time.Second), // Adjust based on your needs
			),
		),
	)

	// Set the global MeterProvider
	otel.SetMeterProvider(mp)

	// Return a shutdown function that cleans up both providers
	shutdown := func(ctx context.Context) error {
		// Shutdown tracer provider
		if err := tp.Shutdown(ctx); err != nil {
			return fmt.Errorf("failed to shutdown tracer provider: %w", err)
		}
		// Shutdown meter provider
		if err := mp.Shutdown(ctx); err != nil {
			return fmt.Errorf("failed to shutdown meter provider: %w", err)
		}
		return nil
	}

	return shutdown, nil
}

// Interceptor creates a Connect interceptor for collecting telemetry and tracing.
//
//nolint:ireturn
func Interceptor(logger *slog.Logger, opts *Options) connect.Interceptor {
	// We use the logger for debugging in case of initialization errors
	logger.Debug("Creating telemetry interceptor")
	// Get a meter from the global MeterProvider
	meter := otel.GetMeterProvider().Meter("dindenault")
	// Get a tracer from the global TracerProvider
	tracer := otel.GetTracerProvider().Tracer("dindenault")

	// Create instruments
	requestCounter, _ := meter.Int64Counter("rpc.requests",
		metric.WithDescription("Number of RPC requests received"),
	)

	responseCounter, _ := meter.Int64Counter("rpc.responses",
		metric.WithDescription("Number of RPC responses sent"),
	)

	durationHistogram, _ := meter.Float64Histogram("rpc.duration_ms",
		metric.WithDescription("Duration of RPC requests in milliseconds"),
		metric.WithUnit("ms"),
	)

	// Context key for start time
	type startTimeKey struct{}

	var startTimeContextKey = startTimeKey{}

	return connect.UnaryInterceptorFunc(func(next connect.UnaryFunc) connect.UnaryFunc {
		return func(ctx context.Context, req connect.AnyRequest) (connect.AnyResponse, error) {
			// Extract service and method information
			procedure := req.Spec().Procedure
			service, method := ExtractServiceAndMethod(procedure)

			// Start a span for this RPC call
			ctx, span := tracer.Start(ctx, fmt.Sprintf("%s.%s", service, method),
				trace.WithAttributes(
					attribute.String("rpc.service", service),
					attribute.String("rpc.method", method),
					attribute.String("rpc.procedure", procedure),
				),
			)
			defer span.End()

			// Get organization from context
			organization := UnknownValue
			if opts != nil && opts.OrganizationFn != nil {
				organization = opts.OrganizationFn(ctx)
			}

			// Add organization to span
			span.SetAttributes(attribute.String("organization", organization))

			// Common attributes for all metrics
			commonAttrs := []attribute.KeyValue{
				attribute.String("service", service),
				attribute.String("method", method),
				attribute.String("organization", organization),
			}

			// Record start time
			startTime := time.Now()
			ctx = context.WithValue(ctx, startTimeContextKey, startTime)

			// Record request metric
			requestCounter.Add(ctx, 1, metric.WithAttributes(commonAttrs...))

			// Call the next handler
			resp, err := next(ctx, req)

			// Determine status code
			status := "success"

			if err != nil {
				// Record error in span
				span.RecordError(err)
				span.SetAttributes(attribute.Bool("error", true))

				var connectErr *connect.Error
				if errors.As(err, &connectErr) {
					status = connectErr.Code().String()
					span.SetAttributes(attribute.String("rpc.connect.status_code", status))
				} else {
					status = "error"
				}
			}

			// Set span status
			span.SetAttributes(attribute.String("rpc.status", status))

			// Response attributes include status
			// Copy commonAttrs and add status
			responseAttrs := make([]attribute.KeyValue, len(commonAttrs)+1)
			copy(responseAttrs, commonAttrs)
			responseAttrs[len(commonAttrs)] = attribute.String("status", status)

			// Record response metric
			responseCounter.Add(ctx, 1, metric.WithAttributes(responseAttrs...))

			// Calculate and record duration
			if startTimeVal := ctx.Value(startTimeContextKey); startTimeVal != nil {
				if startTime, ok := startTimeVal.(time.Time); ok {
					duration := time.Since(startTime)
					durationMs := float64(duration.Milliseconds())
					durationHistogram.Record(ctx, durationMs, metric.WithAttributes(commonAttrs...))
					span.SetAttributes(attribute.Float64("rpc.duration_ms", durationMs))
				}
			}

			return resp, err
		}
	})
}

// InstrumentHandler wraps a Lambda handler with OpenTelemetry instrumentation.
// This is a no-op wrapper since we've removed the aws-lambda-go dependency.
// Users can instrument their handlers manually or through other means if needed.
func InstrumentHandler(handler interface{}) interface{} {
	// Return handler as-is without instrumentation
	return handler
}

// PutCloudWatchMetric sends a custom metric to CloudWatch
func PutCloudWatchMetric(ctx context.Context, cwClient *cloudwatch.Client, namespace, metricName string, value float64, dimensions []types.Dimension) error {
	_, err := cwClient.PutMetricData(ctx, &cloudwatch.PutMetricDataInput{
		Namespace: aws.String(namespace),
		MetricData: []types.MetricDatum{
			{
				MetricName: aws.String(metricName),
				Value:      aws.Float64(value),
				Dimensions: dimensions,
				Timestamp:  aws.Time(time.Now()),
				Unit:       types.StandardUnitCount,
			},
		},
	})

	if err != nil {
		return fmt.Errorf("failed to put CloudWatch metric data: %w", err)
	}

	return nil
}

// CreateDimension creates a CloudWatch dimension (v2 uses types.Dimension)
func CreateDimension(name, value string) types.Dimension {
	return types.Dimension{
		Name:  aws.String(name),
		Value: aws.String(value),
	}
}

// ExtractServiceAndMethod extracts the service name and method name from a Connect RPC procedure path.
// Connect procedure paths are typically in the form "/package.Service/Method".
// This is exported for testing purposes.
func ExtractServiceAndMethod(procedure string) (string, string) {
	parts := strings.Split(procedure, "/")

	// Clean empty parts
	var cleanParts []string

	for _, part := range parts {
		if part != "" {
			cleanParts = append(cleanParts, part)
		}
	}

	// Extract service and method
	service := UnknownValue
	method := UnknownValue

	if len(cleanParts) >= 1 {
		service = cleanParts[0]
	}

	if len(cleanParts) >= 2 {
		method = cleanParts[1]
	}

	return service, method
}
