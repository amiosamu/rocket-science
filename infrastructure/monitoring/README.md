# Rocket Science - Observability Stack

This directory contains the complete observability stack configuration for the Rocket Science microservices platform, implementing production-grade monitoring, tracing, and logging.

## 🏗️ Architecture Overview

Our observability stack follows the **three pillars of observability**:

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│    📊 METRICS   │    │    🔍 TRACES     │    │    📝 LOGS      │
│                 │    │                  │    │                 │
│   Prometheus    │    │     Jaeger       │    │   Elasticsearch │
│   + Grafana     │    │                  │    │   + Kibana      │
└─────────────────┘    └──────────────────┘    └─────────────────┘
                                │
                       ┌────────▼────────┐
                       │ OpenTelemetry   │
                       │   Collector     │
                       │                 │
                       │ • Receives data │
                       │ • Processes     │  
                       │ • Routes        │
                       └─────────────────┘
```

## 🚀 Quick Start

### 1. Start the Full Stack
```bash
docker-compose up -d
```

### 2. Access the UIs
- **Grafana**: http://localhost/grafana (admin/admin123)
- **Jaeger**: http://localhost/jaeger  
- **Kibana**: http://localhost/kibana
- **Prometheus**: http://localhost/prometheus (internal only)

### 3. Generate Test Data
```bash
# Create some test orders to generate telemetry
curl -X POST http://localhost/api/orders \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer YOUR_TOKEN" \
  -d '{
    "items": [
      {"part_id": "engine-v1", "quantity": 1},
      {"part_id": "fuel-tank", "quantity": 2}
    ]
  }'
```

## 📊 Components Deep Dive

### 🔍 OpenTelemetry Collector
**Location**: `otel-collector/otel-collector.yml`

**What it does**:
- Central telemetry data hub
- Collects traces, metrics, and logs from all services
- Processes and enriches data
- Routes to appropriate backends

**Key Features**:
- **Receivers**: OTLP, Prometheus scraping, Host metrics
- **Processors**: Batching, sampling, resource attribution
- **Exporters**: Jaeger (traces), Prometheus (metrics), Elasticsearch (logs)

**Endpoints**:
- gRPC: `otel-collector:4317`
- HTTP: `otel-collector:4318`
- Health: `otel-collector:13133/health`

### 📈 Prometheus
**Location**: `prometheus/prometheus.yml`

**What it does**:
- Time-series metrics storage
- Scrapes metrics from all services
- Powers Grafana dashboards

**Monitored Services**:
- All 6 microservices (ports 2112)
- Envoy Gateway (port 9901)
- Infrastructure (PostgreSQL, MongoDB, Redis, Kafka)
- Monitoring stack itself

**Key Metrics**:
- HTTP request rates and latencies
- gRPC call metrics
- Database connection pools
- Kafka consumer lag
- System resources

### 📊 Grafana
**Location**: `grafana/`

**What it does**:
- Metrics visualization and dashboards
- Alerting and notifications
- Data exploration

**Pre-configured Dashboards**:
1. **System Overview** - High-level platform health
2. **Order Service** - Detailed service metrics
3. **Infrastructure** - Database and messaging metrics

**Datasources**:
- Prometheus (metrics)
- Jaeger (traces)  
- Elasticsearch (logs)

### 🔗 Jaeger
**Location**: `jaeger/jaeger.yml`

**What it does**:
- Distributed tracing storage and visualization
- Service dependency mapping
- Performance bottleneck identification

**Features**:
- **Storage**: In-memory (development) / Elasticsearch (production)
- **Sampling**: Configurable per-service rates
- **Formats**: OTLP, Jaeger native, Zipkin compatible

**Service Sampling Rates**:
- Payment Service: 100% (critical)
- Order Service: 50% (high volume)
- IAM Service: 20%
- Inventory Service: 30%
- Others: 10%

### 📝 Kibana + Elasticsearch
**Location**: `kibana/kibana.yml`

**What it does**:
- Log aggregation, search, and visualization
- Log correlation with traces
- Operational debugging

**Features**:
- **Index Pattern**: `rocket-science-logs-*`
- **Log Fields**: service, level, message, trace_id, user_id
- **Dashboards**: Service-specific log analysis
- **Search**: Full-text search across all logs

## 🔧 Configuration Details

### Service Instrumentation

All services are instrumented with:

```go
// Tracing
tracer, _ := tracing.NewTracer("service-name", "1.0.0", otelEndpoint)
defer tracer.Close()

// Metrics  
metrics, _ := metrics.NewMetrics("service-name")

// Logging
logger, _ := logging.NewServiceLogger("service-name", "1.0.0", "info")
```

### Environment Variables

Key configuration variables:

```bash
# OpenTelemetry
OTEL_ENDPOINT=http://otel-collector:4317
TRACING_ENABLED=true
METRICS_ENABLED=true

# Service identification
SERVICE_NAME=order-service
SERVICE_VERSION=1.0.0

# Logging
LOG_LEVEL=info
LOG_FORMAT=json
```

## 📋 Monitoring Checklist

### ✅ Service Health
- [ ] All services show as "UP" in Grafana
- [ ] Request rates are normal
- [ ] Error rates below 1%
- [ ] Response times under SLA

### ✅ Infrastructure Health  
- [ ] Database connections stable
- [ ] Kafka consumer lag minimal
- [ ] Memory usage within limits
- [ ] No critical errors in logs

### ✅ Distributed Tracing
- [ ] Traces appear in Jaeger
- [ ] Service dependencies visible
- [ ] No broken trace spans
- [ ] Performance bottlenecks identified

## 🚨 Alerting (Future Enhancement)

For production, consider adding:

```yaml
# Prometheus Alerting Rules
groups:
  - name: rocket-science-alerts
    rules:
      - alert: HighErrorRate
        expr: rate(http_requests_total{status=~"5.."}[5m]) > 0.05
        annotations:
          summary: "High error rate detected"
          
      - alert: HighLatency
        expr: histogram_quantile(0.95, http_request_duration_seconds_bucket) > 0.5
        annotations:
          summary: "High latency detected"
```

## 🔍 Troubleshooting

### Common Issues

**1. No Metrics in Grafana**
```bash
# Check Prometheus targets
curl http://localhost:9090/api/v1/targets

# Check service metrics endpoints
curl http://order-service:2112/metrics
```

**2. No Traces in Jaeger**
```bash
# Check OTEL Collector logs
docker logs otel-collector

# Verify service OTEL configuration
curl http://order-service:8080/health
```

**3. No Logs in Kibana**
```bash
# Check Elasticsearch indices
curl http://elasticsearch:9200/_cat/indices

# Check OTEL Collector elasticsearch exporter
docker logs otel-collector | grep elasticsearch
```

### Debug Commands

```bash
# View all monitoring containers
docker ps | grep -E "(prometheus|grafana|jaeger|kibana|otel)"

# Check service health endpoints
for service in iam order payment inventory assembly notification; do
  curl -s http://localhost/api/${service}/health | jq .
done

# Generate test traces
curl -H "X-Trace-Id: $(openssl rand -hex 16)" \
     http://localhost/api/orders
```

## 📚 Additional Resources

- [OpenTelemetry Documentation](https://opentelemetry.io/docs/go/)
- [Prometheus Query Examples](https://prometheus.io/docs/prometheus/latest/querying/examples/)
- [Grafana Dashboard Best Practices](https://grafana.com/docs/grafana/latest/best-practices/)
- [Jaeger Query Syntax](https://www.jaegertracing.io/docs/1.35/frontend-ui/)

## 🎯 Production Considerations

For production deployment:

1. **Security**: Enable authentication on all UIs
2. **Storage**: Use persistent storage for Prometheus and Elasticsearch  
3. **Scaling**: Use Prometheus federation for multi-cluster monitoring
4. **Retention**: Configure appropriate data retention policies
5. **Alerting**: Set up AlertManager with PagerDuty/Slack integration
6. **Backup**: Regular snapshots of dashboards and configurations

---

*This observability stack provides complete visibility into the Rocket Science platform, enabling proactive monitoring, rapid troubleshooting, and data-driven optimization.* 