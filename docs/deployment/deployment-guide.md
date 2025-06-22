# Rocket Science Microservices - Deployment Guide

This guide provides comprehensive instructions for deploying the Rocket Science microservices system in different environments.

## 🚀 Quick Start

For the fastest deployment experience, use our automated scripts:

```bash
# 1. Complete development setup (builds everything)
make dev-setup

# 2. Quick start (if images are already built)
make quick-start
```

## 📋 Prerequisites

### System Requirements
- **Docker** 20.10+ and **Docker Compose** 2.0+
- **Git** for version control
- **Make** (optional, for convenience commands)
- **curl** (for health checks)

### Hardware Requirements

**Minimum (Development):**
- 8GB RAM
- 4 CPU cores
- 20GB disk space

**Recommended (Production):**
- 16GB+ RAM
- 8+ CPU cores
- 100GB+ disk space

## 🏗️ Deployment Options

### 1. Local Development Deployment

**Best for:** Development, testing, learning

```bash
# Method 1: Using Makefile (Recommended)
make deploy-local

# Method 2: Using deployment script
./deployments/scripts/deploy.sh local

# Method 3: Using Docker Compose directly
docker-compose up -d
```

**Features:**
- All services on localhost
- Development-friendly logging
- Hot reload support
- Admin interfaces exposed
- No authentication required for monitoring

### 2. Production Deployment

**Best for:** Production environments, staging

```bash
# First-time setup
make setup  # Creates .env file from template

# Edit .env file with production values
nano .env   # Update passwords, secrets, tokens

# Deploy to production
make deploy-prod

# Or using the script directly
./deployments/scripts/deploy.sh production
```

**Features:**
- Resource limits and reservations
- Production-grade logging
- Security hardening
- Environment-based configuration
- Health checks and monitoring

### 3. Staging Deployment

**Best for:** Pre-production testing

```bash
make deploy-staging
# or
./deployments/scripts/deploy.sh staging
```

## 🔧 Configuration

### Environment Variables

Create a `.env` file from the template:

```bash
cp .env.example .env
```

**Critical Variables to Update:**

```bash
# Database passwords
DB_PASSWORD=your_strong_password_here
MONGO_PASSWORD=your_mongo_password_here
REDIS_PASSWORD=your_redis_password_here

# JWT Secret (make it long and random)
JWT_SECRET=your-super-secret-jwt-key-256-bits-long

# Telegram Bot Token (for notifications)
TELEGRAM_BOT_TOKEN=your_telegram_bot_token

# Monitoring passwords
GRAFANA_ADMIN_PASSWORD=your_grafana_password
KIBANA_PASSWORD=your_kibana_password
```

### Docker Compose Files

The system uses a layered approach:

- `docker-compose.yml` - Base configuration
- `docker-compose.override.yml` - Development overrides
- `deployments/docker/docker-compose.prod.yml` - Production overrides

## 🚀 Deployment Process

### Step-by-Step Deployment

1. **Environment Setup**
   ```bash
   make setup
   # Edit .env file with your values
   ```

2. **Build Images**
   ```bash
   make build-all
   # Or for specific registry
   make build-registry REGISTRY=your-registry.com/rocket-science
   ```

3. **Deploy Services**
   ```bash
   make deploy-local    # For development
   make deploy-prod     # For production
   ```

4. **Verify Deployment**
   ```bash
   make health
   make status
   ```

### Advanced Deployment Options

#### Build Without Cache
```bash
docker-compose build --no-cache
```

#### Deploy Specific Services
```bash
docker-compose up -d envoy iam-service order-service
```

#### Scale Services
```bash
make scale SERVICE=order-service REPLICAS=3
```

#### Deploy with Custom Configuration
```bash
./deployments/scripts/deploy.sh production --no-build --skip-migrations
```

## 🌐 Service Access

After successful deployment, access your services:

### Development Environment
- **API Gateway:** http://localhost
- **Grafana:** http://localhost/grafana (admin:admin123)
- **Jaeger:** http://localhost/jaeger
- **Kibana:** http://localhost/kibana
- **Envoy Admin:** http://localhost:9901

### Production Environment
- **API Gateway:** https://your-domain.com
- **Grafana:** https://your-domain.com/grafana
- **Jaeger:** https://your-domain.com/jaeger
- **Kibana:** https://your-domain.com/kibana

## 🔍 Monitoring & Health Checks

### Health Check Commands

```bash
# Overall system health
make health

# Service status
make status

# View logs
make logs                           # All services
make logs SERVICE=order-service     # Specific service

# Connect to databases
make db-connect      # PostgreSQL
make mongo-connect   # MongoDB
make redis-connect   # Redis
```

### Monitoring Stack

The system includes comprehensive monitoring:

- **Metrics:** Prometheus + Grafana
- **Tracing:** Jaeger
- **Logs:** Elasticsearch + Kibana
- **APM:** OpenTelemetry Collector

## 🛠️ Management Commands

### Service Management

```bash
# Start/stop services
make up          # Start all services
make down        # Stop all services
make restart     # Restart all services

# Restart specific service
make restart-service SERVICE=order-service

# Scale services
make scale SERVICE=payment-service REPLICAS=2

# Open shell in service
make shell SERVICE=iam-service
```

### Database Operations

```bash
# Database connections
make db-connect      # PostgreSQL
make mongo-connect   # MongoDB
make redis-connect   # Redis
```

### Cleanup Operations

```bash
make clean           # Remove containers and volumes
make clean-images    # Remove project images
make clean-all       # Complete cleanup
```

## 🐛 Troubleshooting

### Common Issues

#### Services Won't Start
```bash
# Check Docker daemon
docker info

# Check logs for errors
make logs SERVICE=problematic-service

# Verify configuration
docker-compose config
```

#### Database Connection Issues
```bash
# Check database status
make status

# Restart databases
docker-compose restart postgres mongo redis

# Check database logs
make logs SERVICE=postgres
```

#### Port Conflicts
```bash
# Check port usage
netstat -tlnp | grep :80

# Stop conflicting services
sudo systemctl stop apache2  # or nginx
```

#### Memory Issues
```bash
# Check system resources
docker system df
docker stats

# Clean up unused resources
docker system prune -a
```

### Performance Tuning

#### Database Performance
```bash
# PostgreSQL tuning (in production override)
- shared_buffers=256MB
- effective_cache_size=1GB
- max_connections=200
```

#### Container Resources
```bash
# Adjust resource limits in docker-compose.prod.yml
deploy:
  resources:
    limits:
      cpus: '1'
      memory: 512M
```

## 🔒 Security Considerations

### Production Security Checklist

- [ ] Change all default passwords
- [ ] Use strong JWT secrets
- [ ] Enable TLS/SSL certificates
- [ ] Restrict network access
- [ ] Configure firewall rules
- [ ] Enable audit logging
- [ ] Regular security updates
- [ ] Use secrets management

### Environment Security

```bash
# Never commit .env files
echo ".env" >> .gitignore

# Use proper file permissions
chmod 600 .env

# Consider using Docker secrets
docker secret create jwt_secret jwt_secret.txt
```

## 📦 Backup & Recovery

### Database Backups

```bash
# PostgreSQL backup
docker exec postgres pg_dump -U rocket_user rocket_science > backup.sql

# MongoDB backup
docker exec mongodb mongodump --out /backup

# Redis backup
docker exec redis redis-cli --rdb /backup/dump.rdb
```

### Automated Backups

Set up automated backups using cron:

```bash
# Add to crontab
0 2 * * * /path/to/backup-script.sh
```

## 🔄 Updates & Maintenance

### Rolling Updates

```bash
# Update specific service
docker-compose build order-service
docker-compose up -d --no-deps order-service

# Zero-downtime deployment
./deployments/scripts/rolling-update.sh
```

### Maintenance Mode

```bash
# Enable maintenance mode
docker-compose scale envoy=0

# Perform maintenance
# ...

# Re-enable services
docker-compose scale envoy=1
```

## 🆘 Support & Resources

### Getting Help

1. **Check Logs:** `make logs SERVICE=service-name`
2. **System Status:** `make health`
3. **Documentation:** Review service-specific README files
4. **Issues:** Create GitHub issues for bugs

### Useful Resources

- [Docker Compose Documentation](https://docs.docker.com/compose/)
- [Envoy Proxy Documentation](https://www.envoyproxy.io/docs/)
- [Prometheus Monitoring](https://prometheus.io/docs/)
- [Grafana Dashboards](https://grafana.com/docs/)

---

## 📝 Quick Reference

### Most Common Commands

```bash
# Development
make dev-setup          # Complete setup
make deploy-local       # Local deployment
make logs              # View all logs
make health            # Check health

# Production
make setup             # Environment setup
make deploy-prod       # Production deployment
make status            # Service status
make clean             # Cleanup

# Monitoring
make logs SERVICE=name  # Service logs
make shell SERVICE=name # Service shell
make db-connect        # Database access
```

This completes the deployment guide. Your Rocket Science microservices system is ready for deployment! 🚀 