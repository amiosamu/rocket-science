package logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

// Logger defines the interface for logging
type Logger interface {
	Debug(ctx context.Context, message string, fields ...map[string]interface{})
	Info(ctx context.Context, message string, fields ...map[string]interface{})
	Warn(ctx context.Context, message string, fields ...map[string]interface{})
	Error(ctx context.Context, message string, err error, fields ...map[string]interface{})
	With(fields map[string]interface{}) Logger
}

// SlogLogger implements Logger using the standard slog package
type SlogLogger struct {
	logger *slog.Logger
	fields map[string]interface{}
}

// NewLogger creates a new logger with the specified level
func NewLogger(level string) (Logger, error) {
	var slogLevel slog.Level
	
	switch strings.ToLower(level) {
	case "debug":
		slogLevel = slog.LevelDebug
	case "info":
		slogLevel = slog.LevelInfo
	case "warn", "warning":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}

	// Create a JSON handler for structured logging
	handler := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slogLevel,
		AddSource: true,
	})

	logger := slog.New(handler)

	return &SlogLogger{
		logger: logger,
		fields: make(map[string]interface{}),
	}, nil
}

// Debug logs a debug message
func (l *SlogLogger) Debug(ctx context.Context, message string, fields ...map[string]interface{}) {
	l.log(ctx, slog.LevelDebug, message, nil, fields...)
}

// Info logs an info message
func (l *SlogLogger) Info(ctx context.Context, message string, fields ...map[string]interface{}) {
	l.log(ctx, slog.LevelInfo, message, nil, fields...)
}

// Warn logs a warning message
func (l *SlogLogger) Warn(ctx context.Context, message string, fields ...map[string]interface{}) {
	l.log(ctx, slog.LevelWarn, message, nil, fields...)
}

// Error logs an error message
func (l *SlogLogger) Error(ctx context.Context, message string, err error, fields ...map[string]interface{}) {
	l.log(ctx, slog.LevelError, message, err, fields...)
}

// With creates a new logger with additional fields
func (l *SlogLogger) With(fields map[string]interface{}) Logger {
	newFields := make(map[string]interface{})
	
	// Copy existing fields
	for k, v := range l.fields {
		newFields[k] = v
	}
	
	// Add new fields
	for k, v := range fields {
		newFields[k] = v
	}
	
	return &SlogLogger{
		logger: l.logger,
		fields: newFields,
	}
}

// log is the internal logging method
func (l *SlogLogger) log(ctx context.Context, level slog.Level, message string, err error, fields ...map[string]interface{}) {
	// Build attributes from fields
	var attrs []slog.Attr
	
	// Add persistent fields
	for k, v := range l.fields {
		attrs = append(attrs, slog.Any(k, v))
	}
	
	// Add provided fields
	for _, fieldMap := range fields {
		for k, v := range fieldMap {
			attrs = append(attrs, slog.Any(k, v))
		}
	}
	
	// Add error if provided
	if err != nil {
		attrs = append(attrs, slog.String("error", err.Error()))
	}
	
	// Add trace ID if available in context
	if traceID := getTraceIDFromContext(ctx); traceID != "" {
		attrs = append(attrs, slog.String("trace_id", traceID))
	}
	
	// Add request ID if available in context
	if requestID := getRequestIDFromContext(ctx); requestID != "" {
		attrs = append(attrs, slog.String("request_id", requestID))
	}

	// Log the message
	l.logger.LogAttrs(ctx, level, message, attrs...)
}

// Context helper functions

// getTraceIDFromContext extracts trace ID from context
func getTraceIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	
	// Try to get trace ID from context
	if traceID, ok := ctx.Value("trace_id").(string); ok {
		return traceID
	}
	
	// Could also extract from OpenTelemetry span context here
	// span := trace.SpanFromContext(ctx)
	// if span.SpanContext().IsValid() {
	//     return span.SpanContext().TraceID().String()
	// }
	
	return ""
}

// getRequestIDFromContext extracts request ID from context
func getRequestIDFromContext(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	
	if requestID, ok := ctx.Value("request_id").(string); ok {
		return requestID
	}
	
	return ""
}

// NoOpLogger is a logger that does nothing (useful for testing)
type NoOpLogger struct{}

// NewNoOpLogger creates a no-op logger
func NewNoOpLogger() Logger {
	return &NoOpLogger{}
}

func (n *NoOpLogger) Debug(ctx context.Context, message string, fields ...map[string]interface{}) {}
func (n *NoOpLogger) Info(ctx context.Context, message string, fields ...map[string]interface{})  {}
func (n *NoOpLogger) Warn(ctx context.Context, message string, fields ...map[string]interface{})  {}
func (n *NoOpLogger) Error(ctx context.Context, message string, err error, fields ...map[string]interface{}) {}
func (n *NoOpLogger) With(fields map[string]interface{}) Logger { return n }

// Helper function to create a logger with service context
func NewServiceLogger(serviceName, serviceVersion, logLevel string) (Logger, error) {
	logger, err := NewLogger(logLevel)
	if err != nil {
		return nil, fmt.Errorf("failed to create logger: %w", err)
	}
	
	return logger.With(map[string]interface{}{
		"service":         serviceName,
		"service_version": serviceVersion,
	}), nil
}