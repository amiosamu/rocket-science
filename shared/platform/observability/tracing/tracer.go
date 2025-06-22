package tracing

import (
	"context"
	"fmt"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.21.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// Tracer wraps OpenTelemetry functionality
type Tracer interface {
	Start(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span)
	Close() error
	GetTracer(name string) trace.Tracer
}

// OTelTracer implements Tracer using OpenTelemetry
type OTelTracer struct {
	provider   *sdktrace.TracerProvider
	serviceName string
}

// TracerConfig holds configuration for tracing
type TracerConfig struct {
	ServiceName    string
	ServiceVersion string
	Environment    string
	OTELEndpoint   string
	SamplingRatio  float64
	Enabled        bool
}

// NewTracer creates a new OpenTelemetry tracer
func NewTracer(serviceName, serviceVersion, otelEndpoint string) (Tracer, error) {
	config := TracerConfig{
		ServiceName:    serviceName,
		ServiceVersion: serviceVersion,
		Environment:    "development", // Could be configurable
		OTELEndpoint:   otelEndpoint,
		SamplingRatio:  1.0, // Sample all traces in development
		Enabled:        otelEndpoint != "",
	}

	return NewTracerWithConfig(config)
}

// NewTracerWithConfig creates a new tracer with detailed configuration
func NewTracerWithConfig(config TracerConfig) (Tracer, error) {
	if !config.Enabled {
		return NewNoOpTracer(), nil
	}

	// Create OTLP exporter
	exporter, err := createOTLPExporter(config.OTELEndpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
	}

	// Create resource with service information
	res, err := createResource(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Create trace provider
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
		sdktrace.WithSampler(sdktrace.TraceIDRatioBased(config.SamplingRatio)),
	)

	// Set global provider
	otel.SetTracerProvider(provider)

	// Set global propagator for trace context propagation
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))

	return &OTelTracer{
		provider:    provider,
		serviceName: config.ServiceName,
	}, nil
}

// Start starts a new span
func (t *OTelTracer) Start(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	tracer := otel.Tracer(t.serviceName)
	return tracer.Start(ctx, spanName, opts...)
}

// GetTracer returns a tracer for the given name
func (t *OTelTracer) GetTracer(name string) trace.Tracer {
	return otel.Tracer(name)
}

// Close shuts down the tracer provider
func (t *OTelTracer) Close() error {
	if t.provider != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return t.provider.Shutdown(ctx)
	}
	return nil
}

// createOTLPExporter creates an OTLP gRPC exporter
func createOTLPExporter(endpoint string) (sdktrace.SpanExporter, error) {
	// Create gRPC connection to OTEL collector
	conn, err := grpc.Dial(endpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to OTEL collector: %w", err)
	}

	// Create OTLP trace exporter
	exporter, err := otlptracegrpc.New(
		context.Background(),
		otlptracegrpc.WithGRPCConn(conn),
	)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
	}

	return exporter, nil
}

// createResource creates a resource with service metadata
func createResource(config TracerConfig) (*resource.Resource, error) {
	return resource.Merge(
		resource.Default(),
		resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(config.ServiceName),
			semconv.ServiceVersionKey.String(config.ServiceVersion),
			semconv.DeploymentEnvironmentKey.String(config.Environment),
			attribute.String("service.namespace", "rocket-science"),
		),
	)
}

// NoOpTracer is a tracer that does nothing (useful when tracing is disabled)
type NoOpTracer struct{}

// NewNoOpTracer creates a no-op tracer
func NewNoOpTracer() Tracer {
	return &NoOpTracer{}
}

func (n *NoOpTracer) Start(ctx context.Context, spanName string, opts ...trace.SpanStartOption) (context.Context, trace.Span) {
	return ctx, trace.SpanFromContext(ctx)
}

func (n *NoOpTracer) GetTracer(name string) trace.Tracer {
	return otel.Tracer(name)
}

func (n *NoOpTracer) Close() error {
	return nil
}

// Helper functions for span management

// StartSpan is a convenience function to start a new span
func StartSpan(ctx context.Context, tracer trace.Tracer, spanName string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	opts := []trace.SpanStartOption{}
	if len(attrs) > 0 {
		opts = append(opts, trace.WithAttributes(attrs...))
	}
	return tracer.Start(ctx, spanName, opts...)
}

// AddSpanAttributes adds attributes to the current span
func AddSpanAttributes(ctx context.Context, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.SetAttributes(attrs...)
	}
}

// AddSpanEvent adds an event to the current span
func AddSpanEvent(ctx context.Context, name string, attrs ...attribute.KeyValue) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.AddEvent(name, trace.WithAttributes(attrs...))
	}
}

// RecordError records an error on the current span
func RecordError(ctx context.Context, err error) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() && err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

// FinishSpan finishes the current span with success status
func FinishSpan(ctx context.Context) {
	span := trace.SpanFromContext(ctx)
	if span.IsRecording() {
		span.SetStatus(codes.Ok, "")
		span.End()
	}
}

// GetTraceID extracts the trace ID from context
func GetTraceID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		return span.SpanContext().TraceID().String()
	}
	return ""
}

// GetSpanID extracts the span ID from context
func GetSpanID(ctx context.Context) string {
	span := trace.SpanFromContext(ctx)
	if span.SpanContext().IsValid() {
		return span.SpanContext().SpanID().String()
	}
	return ""
}

// InjectTraceContext injects trace context into a map (for HTTP headers, etc.)
func InjectTraceContext(ctx context.Context, carrier propagation.TextMapCarrier) {
	otel.GetTextMapPropagator().Inject(ctx, carrier)
}

// ExtractTraceContext extracts trace context from a map (from HTTP headers, etc.)
func ExtractTraceContext(ctx context.Context, carrier propagation.TextMapCarrier) context.Context {
	return otel.GetTextMapPropagator().Extract(ctx, carrier)
}

// SpanProcessor is a custom span processor for additional processing
type SpanProcessor struct {
	next sdktrace.SpanProcessor
}

// NewSpanProcessor creates a new span processor
func NewSpanProcessor(next sdktrace.SpanProcessor) *SpanProcessor {
	return &SpanProcessor{next: next}
}

func (p *SpanProcessor) OnStart(parent context.Context, s sdktrace.ReadWriteSpan) {
	// Add custom logic here if needed
	if p.next != nil {
		p.next.OnStart(parent, s)
	}
}

func (p *SpanProcessor) OnEnd(s sdktrace.ReadOnlySpan) {
	// Add custom logic here if needed
	if p.next != nil {
		p.next.OnEnd(s)
	}
}

func (p *SpanProcessor) Shutdown(ctx context.Context) error {
	if p.next != nil {
		return p.next.Shutdown(ctx)
	}
	return nil
}

func (p *SpanProcessor) ForceFlush(ctx context.Context) error {
	if p.next != nil {
		return p.next.ForceFlush(ctx)
	}
	return nil
}

// TracingMiddleware creates HTTP middleware for automatic span creation
type TracingMiddleware struct {
	tracer      trace.Tracer
	serviceName string
}

// NewTracingMiddleware creates a new tracing middleware
func NewTracingMiddleware(serviceName string) *TracingMiddleware {
	return &TracingMiddleware{
		tracer:      otel.Tracer(serviceName),
		serviceName: serviceName,
	}
}

// Common attribute keys for consistent span tagging
var (
	HTTPMethodKey     = attribute.Key("http.method")
	HTTPURLKey        = attribute.Key("http.url")
	HTTPStatusCodeKey = attribute.Key("http.status_code")
	HTTPUserAgentKey  = attribute.Key("http.user_agent")
	HTTPRemoteAddrKey = attribute.Key("http.remote_addr")
	
	GRPCMethodKey     = attribute.Key("grpc.method")
	GRPCServiceKey    = attribute.Key("grpc.service")
	GRPCStatusCodeKey = attribute.Key("grpc.status_code")
	
	DBOperationKey = attribute.Key("db.operation")
	DBTableKey     = attribute.Key("db.table")
	DBStatementKey = attribute.Key("db.statement")
	
	OrderIDKey = attribute.Key("order.id")
	UserIDKey  = attribute.Key("user.id")
)