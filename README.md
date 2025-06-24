# Rocket Science - Microservices Platform

A comprehensive microservices learning project that demonstrates enterprise-grade architecture patterns and technologies.

## 🚀 What You'll Learn

- **Develop 6 microservices** connected through Kafka and gRPC, isolated from the external world using Envoy Gateway
- **Implement monitoring** following OpenTelemetry standards with integration and e2e tests to eliminate operational errors
- **Master data caching** with Redis and asynchronous microservice communication with Kafka
- **Work with PostgreSQL** by creating custom platform components that simplify development
- **Implement inter-service communication**, authentication and authorization systems
- **Apply architectural approaches** for building microservices in practice

## 📚 Learning Modules

### Module 1: Core APIs
- Implement HTTP API for Order Service according to contract
- Implement gRPC API for Inventory Service and Payment Service
- Integrate gRPC clients in Order Service with proper Inventory and Payment call logic

### Module 2: Architecture & Testing
- Align Order, Inventory, Payment services architecture with best practices
- Configure test coverage reporting in each service's README.md
- Ensure minimum 40% unit test coverage across all three services

### Module 3: Data Persistence
- Create docker-compose for Order Service and Inventory Service with PostgreSQL and MongoDB
- Replace in-memory maps with PostgreSQL in Order Service with full order table migrations
- Migrate Inventory Service from maps to MongoDB
- Update unit tests when data handling logic changes

### Module 4: Configuration & Platform
- Implement environment variable configuration across all three services
- Integrate DI container in Order, Inventory, Payment services
- Write at least one integration test for repository in Order Service
- Write at least one e2e test for gRPC API in Inventory Service
- Implement platform library with logger wrapper and integrate across services

### Module 5: Event-Driven Architecture
- Deploy Kafka in KRaft mode with single broker via Docker Compose
- Create Assembly Service following the same architectural style (configuration, DI, layers)

**Assembly Service:**
- Implement Kafka Consumer for order payment events
- Add 10-second processing delay
- Send successful assembly completion event

**Order Service:**
- Implement Kafka Producer sending payment events
- Implement Kafka Consumer for assembly events and update order status

**Notification Service:**
- Create service following same architectural style
- Implement Kafka Consumer listening to payment and assembly events
- Send notifications to Telegram

### Module 6: Identity & Access Management
- Create IAM Service following same architectural style
- Implement gRPC API for IAM Service according to auth service contracts
- Implement session validation interceptor in Inventory Service
- Store user data in PostgreSQL with startup migrations
- Store session data in Redis with 24-hour TTL using keys and hash structures
- Add IAM Service integration in Notification Service to get user contact info (e.g., Telegram ID)

### Module 7: Observability
- Configure log storage for all services and display in Kibana

**Metrics Collection:**
- Order Service: order count and total revenue
- Assembly Service: rocket assembly time

**Alerting:**
- Configure alert: if more than 10 orders per minute, send Telegram notification
- Add request tracing in Order Service: from entry to Payment Service and Inventory Service calls
- Extract span creation logic and other observability tools to platform library

### Module 8: Gateway & Deployment
- Configure Envoy as single system entry point
- Define routes to services: HTTP and gRPC
- Add session validation through IAM service for each incoming request (Lua script)
- Hide all services behind Envoy - expose only one external port
- Containerize all services with Docker and integrate via Docker Compose

## 🏗️ Project Architecture

![Project Architecture](Project.png "Rocket Science Architecture")

## 🚀 Quick Start

1. **Clone the repository**
   ```bash
   git clone <repository-url>
   cd rocket-science
   ```

2. **Setup environment**
   ```bash
   cp .env.example .env
   # Edit .env with your configuration
   ```

3. **Start the platform**
   ```bash
   make up
   ```

4. **Verify deployment**
   ```bash
   make health-check
   ```

## 🛠️ Technology Stack

- **Languages:** Go
- **Databases:** PostgreSQL, MongoDB, Redis
- **Messaging:** Apache Kafka
- **API:** gRPC, HTTP/REST
- **Gateway:** Envoy Proxy
- **Monitoring:** Prometheus, Grafana, Jaeger, Kibana
- **Containerization:** Docker, Docker Compose
- **Testing:** Unit, Integration, E2E tests

## 📁 Project Structure
