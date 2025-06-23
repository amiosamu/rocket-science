# Envoy Gateway Configuration

Envoy serves as the centralized API Gateway for the Rocket Science microservices system, providing a single entry point for all external traffic.

## 🌟 Features

- **Single Entry Point**: All external traffic goes through port 80
- **Authentication**: Automatic session validation via Lua script
- **Load Balancing**: Round-robin load balancing for all services
- **Health Checks**: Automatic health monitoring of backend services
- **Protocol Support**: Both HTTP and gRPC routing
- **Monitoring Integration**: Routes for Grafana, Jaeger, Kibana, and Prometheus

## 🏗️ Architecture

```
Internet → Envoy (Port 80) → Backend Services
                ↓
            IAM Service (Auth)
```

## 📋 Routing Configuration

### HTTP Routes
- `GET /health` - Health check endpoint (no auth)
- `POST /api/orders/*` - Order Service (requires auth)

### gRPC Routes
- `/iam.IAMService/*` - IAM Service (mixed auth)
- `/payment.PaymentService/*` - Payment Service (requires auth)
- `/inventory.InventoryService/*` - Inventory Service (requires auth)

### Monitoring Routes
- `/grafana` - Grafana dashboard
- `/jaeger` - Jaeger tracing UI
- `/kibana` - Kibana logs UI
- `/prometheus` - Prometheus metrics UI

## 🔐 Authentication Flow

1. **Extract Token**: From `Authorization: Bearer <token>` header or `session_token` cookie
2. **Validate Session**: Call IAM service `/validate-session` endpoint
3. **Add Headers**: Forward user info to backend services:
   - `x-user-id`: User ID
   - `x-user-email`: User email
   - `x-user-role`: User role
   - `x-session-token`: Original session token

## 🚀 Usage Examples

### HTTP API Call
```bash
# With Bearer token
curl -H "Authorization: Bearer your-session-token" \
     http://localhost/api/orders

# With cookie
curl -H "Cookie: session_token=your-session-token" \
     http://localhost/api/orders
```

### gRPC Call
```bash
# Using grpcurl
grpcurl -H "authorization: Bearer your-session-token" \
        localhost:80 \
        payment.PaymentService/ProcessPayment
```

### Access Monitoring Tools
```bash
# Grafana
http://localhost/grafana

# Jaeger
http://localhost/jaeger

# Kibana  
http://localhost/kibana

# Prometheus
http://localhost/prometheus
```

## 🔧 Configuration Files

- `envoy.yaml` - Main Envoy configuration
- `lua/auth_check.lua` - Authentication Lua script
- `Dockerfile` - Envoy container image

## 🏥 Health Checks

Envoy automatically monitors backend service health:

- **HTTP Services**: `GET /health`
- **gRPC Services**: gRPC health check protocol
- **Check Interval**: 10 seconds
- **Timeout**: 5 seconds
- **Unhealthy Threshold**: 3 failures
- **Healthy Threshold**: 2 successes

## 📊 Admin Interface

Envoy admin interface is available at `http://localhost:9901` (in development):

- `/ready` - Readiness check
- `/stats` - Runtime statistics
- `/clusters` - Cluster status
- `/config_dump` - Current configuration

## 🔍 Debugging

### View Envoy Logs
```bash
docker-compose logs -f envoy
```

### Check Cluster Status
```bash
curl http://localhost:9901/clusters
```

### View Configuration
```bash
curl http://localhost:9901/config_dump
```

### Test Authentication
```bash
# Should return 401
curl -v http://localhost/api/orders

# Should work with valid token
curl -H "Authorization: Bearer valid-token" \
     http://localhost/api/orders
```

## 🛡️ Security Considerations

### Development
- Admin interface exposed on port 9901
- Detailed error messages in responses
- Logging includes request details

### Production
- Remove admin interface port exposure
- Generic error messages
- Reduced logging verbosity
- Add rate limiting
- Add CORS policies
- Enable TLS termination

## 🔧 Customization

### Adding New Routes
1. Update `envoy.yaml` routes section
2. Add corresponding cluster configuration
3. Configure authentication requirements
4. Rebuild and restart Envoy

### Modifying Authentication
1. Edit `lua/auth_check.lua`
2. Update public endpoints list
3. Modify token extraction logic
4. Customize user info headers

### Load Balancing
- Default: Round-robin
- Available: LEAST_REQUEST, RING_HASH, RANDOM
- Configure in cluster `lb_policy` section

## 📈 Monitoring

Envoy provides rich metrics through:
- Prometheus metrics endpoint
- Access logs
- Health check status
- Request tracing headers

All metrics are automatically collected by the monitoring stack. 