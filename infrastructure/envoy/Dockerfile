FROM envoyproxy/envoy:v1.29-latest

# Install curl for health checks and other utilities
RUN apt-get update && \
    apt-get install -y curl && \
    apt-get clean && \
    rm -rf /var/lib/apt/lists/*

# Create directories for configuration
RUN mkdir -p /etc/envoy/lua

# Copy Envoy configuration
COPY envoy.yaml /etc/envoy/envoy.yaml

# Copy Lua scripts
COPY lua/ /etc/envoy/lua/

# Set proper permissions
RUN chmod 644 /etc/envoy/envoy.yaml && \
    chmod 644 /etc/envoy/lua/*.lua

# Expose the listener port and admin port
EXPOSE 10000 9901

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=30s --retries=3 \
    CMD curl -f http://localhost:9901/ready || exit 1

# Run Envoy with the configuration
CMD ["/usr/local/bin/envoy", "-c", "/etc/envoy/envoy.yaml", "--service-cluster", "envoy-gateway"]
