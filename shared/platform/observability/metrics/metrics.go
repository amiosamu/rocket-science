package metrics

import (
	"fmt"
	"sync"
	"time"
)

// Metrics defines the interface for metrics collection
type Metrics interface {
	IncrementCounter(name string, labels map[string]string)
	RecordValue(name string, value float64, labels map[string]string)
	RecordDuration(name string, duration time.Duration, labels map[string]string)
	SetGauge(name string, value float64, labels map[string]string)
}

// InMemoryMetrics implements Metrics interface with in-memory storage
// This is a simple implementation for development/testing
// In production, you'd use Prometheus, StatsD, or other metrics systems
type InMemoryMetrics struct {
	serviceName string
	counters    map[string]*Counter
	gauges      map[string]*Gauge
	histograms  map[string]*Histogram
	mu          sync.RWMutex
}

// Counter represents a counter metric
type Counter struct {
	Name   string            `json:"name"`
	Help   string            `json:"help"`
	Labels map[string]string `json:"labels"`
	Value  int64             `json:"value"`
}

// Gauge represents a gauge metric
type Gauge struct {
	Name   string            `json:"name"`
	Help   string            `json:"help"`
	Labels map[string]string `json:"labels"`
	Value  float64           `json:"value"`
}

// Histogram represents a histogram metric
type Histogram struct {
	Name    string            `json:"name"`
	Help    string            `json:"help"`
	Labels  map[string]string `json:"labels"`
	Count   int64             `json:"count"`
	Sum     float64           `json:"sum"`
	Buckets map[string]int64  `json:"buckets"`
}

// NewMetrics creates a new metrics instance
func NewMetrics(serviceName string) (Metrics, error) {
	return &InMemoryMetrics{
		serviceName: serviceName,
		counters:    make(map[string]*Counter),
		gauges:      make(map[string]*Gauge),
		histograms:  make(map[string]*Histogram),
	}, nil
}

// IncrementCounter increments a counter metric
func (m *InMemoryMetrics) IncrementCounter(name string, labels map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	key := m.metricKey(name, labels)
	
	if counter, exists := m.counters[key]; exists {
		counter.Value++
	} else {
		m.counters[key] = &Counter{
			Name:   name,
			Labels: m.copyLabels(labels),
			Value:  1,
		}
	}
}

// RecordValue records a value for a histogram metric
func (m *InMemoryMetrics) RecordValue(name string, value float64, labels map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	key := m.metricKey(name, labels)
	
	if histogram, exists := m.histograms[key]; exists {
		histogram.Count++
		histogram.Sum += value
	} else {
		m.histograms[key] = &Histogram{
			Name:    name,
			Labels:  m.copyLabels(labels),
			Count:   1,
			Sum:     value,
			Buckets: make(map[string]int64),
		}
	}
}

// RecordDuration records a duration for a histogram metric
func (m *InMemoryMetrics) RecordDuration(name string, duration time.Duration, labels map[string]string) {
	m.RecordValue(name, duration.Seconds(), labels)
}

// SetGauge sets a gauge metric value
func (m *InMemoryMetrics) SetGauge(name string, value float64, labels map[string]string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	key := m.metricKey(name, labels)
	
	m.gauges[key] = &Gauge{
		Name:   name,
		Labels: m.copyLabels(labels),
		Value:  value,
	}
}

// GetMetrics returns all collected metrics (useful for debugging/monitoring endpoints)
func (m *InMemoryMetrics) GetMetrics() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	return map[string]interface{}{
		"service":    m.serviceName,
		"counters":   m.copyCounters(),
		"gauges":     m.copyGauges(),
		"histograms": m.copyHistograms(),
	}
}

// Helper methods

func (m *InMemoryMetrics) metricKey(name string, labels map[string]string) string {
	key := name
	if labels != nil {
		for k, v := range labels {
			key += fmt.Sprintf("_%s_%s", k, v)
		}
	}
	return key
}

func (m *InMemoryMetrics) copyLabels(labels map[string]string) map[string]string {
	if labels == nil {
		return nil
	}
	
	copy := make(map[string]string)
	for k, v := range labels {
		copy[k] = v
	}
	return copy
}

func (m *InMemoryMetrics) copyCounters() map[string]*Counter {
	copy := make(map[string]*Counter)
	for k, v := range m.counters {
		copy[k] = &Counter{
			Name:   v.Name,
			Labels: m.copyLabels(v.Labels),
			Value:  v.Value,
		}
	}
	return copy
}

func (m *InMemoryMetrics) copyGauges() map[string]*Gauge {
	copy := make(map[string]*Gauge)
	for k, v := range m.gauges {
		copy[k] = &Gauge{
			Name:   v.Name,
			Labels: m.copyLabels(v.Labels),
			Value:  v.Value,
		}
	}
	return copy
}

func (m *InMemoryMetrics) copyHistograms() map[string]*Histogram {
	copy := make(map[string]*Histogram)
	for k, v := range m.histograms {
		buckets := make(map[string]int64)
		for bk, bv := range v.Buckets {
			buckets[bk] = bv
		}
		
		copy[k] = &Histogram{
			Name:    v.Name,
			Labels:  m.copyLabels(v.Labels),
			Count:   v.Count,
			Sum:     v.Sum,
			Buckets: buckets,
		}
	}
	return copy
}

// NoOpMetrics is a metrics implementation that does nothing (useful for testing)
type NoOpMetrics struct{}

// NewNoOpMetrics creates a no-op metrics instance
func NewNoOpMetrics() Metrics {
	return &NoOpMetrics{}
}

func (n *NoOpMetrics) IncrementCounter(name string, labels map[string]string)                    {}
func (n *NoOpMetrics) RecordValue(name string, value float64, labels map[string]string)         {}
func (n *NoOpMetrics) RecordDuration(name string, duration time.Duration, labels map[string]string) {}
func (n *NoOpMetrics) SetGauge(name string, value float64, labels map[string]string)            {}

// Timer is a helper for timing operations
type Timer struct {
	metrics Metrics
	name    string
	labels  map[string]string
	start   time.Time
}

// StartTimer starts a new timer
func StartTimer(metrics Metrics, name string, labels map[string]string) *Timer {
	return &Timer{
		metrics: metrics,
		name:    name,
		labels:  labels,
		start:   time.Now(),
	}
}

// Stop stops the timer and records the duration
func (t *Timer) Stop() {
	duration := time.Since(t.start)
	t.metrics.RecordDuration(t.name, duration, t.labels)
}