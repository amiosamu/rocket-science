package redis

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/amiosamu/rocket-science/shared/platform/errors"
	"github.com/amiosamu/rocket-science/shared/platform/observability/logging"
)

// Config holds Redis connection configuration
type Config struct {
	Host         string        `json:"host"`
	Port         int           `json:"port"`
	Password     string        `json:"password"`
	DB           int           `json:"db"`
	PoolSize     int           `json:"pool_size"`
	MinIdleConns int           `json:"min_idle_conns"`
	DialTimeout  time.Duration `json:"dial_timeout"`
	ReadTimeout  time.Duration `json:"read_timeout"`
	WriteTimeout time.Duration `json:"write_timeout"`
	IdleTimeout  time.Duration `json:"idle_timeout"`
	MaxRetries   int           `json:"max_retries"`
	TLSEnabled   bool          `json:"tls_enabled"`
}

// DefaultConfig returns a default Redis configuration
func DefaultConfig() Config {
	return Config{
		Host:         "localhost",
		Port:         6379,
		Password:     "",
		DB:           0,
		PoolSize:     10,
		MinIdleConns: 5,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		IdleTimeout:  5 * time.Minute,
		MaxRetries:   3,
		TLSEnabled:   false,
	}
}

// Address returns the Redis address string
func (c Config) Address() string {
	return fmt.Sprintf("%s:%d", c.Host, c.Port)
}

// Connection manages a Redis database connection
type Connection struct {
	Client *redis.Client
	config Config
	logger logging.Logger
}

// NewConnection creates a new Redis connection
func NewConnection(config Config, logger logging.Logger) (*Connection, error) {
	// Configure Redis client options
	rdb := redis.NewClient(&redis.Options{
		Addr:            config.Address(),
		Password:        config.Password,
		DB:              config.DB,
		PoolSize:        config.PoolSize,
		MinIdleConns:    config.MinIdleConns,
		DialTimeout:     config.DialTimeout,
		ReadTimeout:     config.ReadTimeout,
		WriteTimeout:    config.WriteTimeout,
		ConnMaxIdleTime: config.IdleTimeout, // This is the correct field name in go-redis v9
		MaxRetries:      config.MaxRetries,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), config.DialTimeout)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		rdb.Close()
		return nil, errors.Wrap(err, "failed to connect to Redis")
	}

	logger.Info(ctx, "Redis connection established", map[string]interface{}{
		"address":        config.Address(),
		"db":             config.DB,
		"pool_size":      config.PoolSize,
		"min_idle_conns": config.MinIdleConns,
		"dial_timeout":   config.DialTimeout,
	})

	return &Connection{
		Client: rdb,
		config: config,
		logger: logger,
	}, nil
}

// Close closes the Redis connection
func (c *Connection) Close() error {
	if c.Client != nil {
		err := c.Client.Close()
		if err != nil {
			c.logger.Error(nil, "Failed to close Redis connection", err)
			return err
		}
		c.logger.Info(nil, "Redis connection closed")
	}
	return nil
}

// HealthCheck performs a health check on the Redis connection
func (c *Connection) HealthCheck(ctx context.Context) error {
	if c.Client == nil {
		return errors.NewInternal("Redis client is nil")
	}

	// Check if we can ping Redis
	if err := c.Client.Ping(ctx).Err(); err != nil {
		return errors.Wrap(err, "Redis ping failed")
	}

	// Test a simple operation
	testKey := "health_check_" + fmt.Sprintf("%d", time.Now().UnixNano())
	testValue := "ok"

	// Set and get a test value
	if err := c.Client.Set(ctx, testKey, testValue, time.Second).Err(); err != nil {
		return errors.Wrap(err, "Redis set operation failed")
	}

	result, err := c.Client.Get(ctx, testKey).Result()
	if err != nil {
		return errors.Wrap(err, "Redis get operation failed")
	}

	if result != testValue {
		return errors.NewInternal("Redis value mismatch")
	}

	// Clean up test key
	c.Client.Del(ctx, testKey)

	return nil
}

// GetStats returns Redis connection and server statistics
func (c *Connection) GetStats(ctx context.Context) map[string]interface{} {
	if c.Client == nil {
		return map[string]interface{}{
			"status": "disconnected",
		}
	}

	stats := map[string]interface{}{
		"status":         "connected",
		"address":        c.config.Address(),
		"db":             c.config.DB,
		"pool_size":      c.config.PoolSize,
		"min_idle_conns": c.config.MinIdleConns,
	}

	// Get pool stats
	poolStats := c.Client.PoolStats()
	stats["pool_stats"] = map[string]interface{}{
		"hits":        poolStats.Hits,
		"misses":      poolStats.Misses,
		"timeouts":    poolStats.Timeouts,
		"total_conns": poolStats.TotalConns,
		"idle_conns":  poolStats.IdleConns,
		"stale_conns": poolStats.StaleConns,
	}

	// Get Redis info
	info, err := c.Client.Info(ctx, "server", "memory", "stats").Result()
	if err == nil {
		stats["server_info"] = parseRedisInfo(info)
	}

	return stats
}

// Set sets a key-value pair with optional expiration
func (c *Connection) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) error {
	err := c.Client.Set(ctx, key, value, expiration).Err()
	if err != nil {
		return errors.Wrap(err, "Redis set operation failed")
	}
	return nil
}

// Get retrieves a value by key
func (c *Connection) Get(ctx context.Context, key string) (string, error) {
	result, err := c.Client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", errors.NewNotFound("key not found")
	}
	if err != nil {
		return "", errors.Wrap(err, "Redis get operation failed")
	}
	return result, nil
}

// Del deletes one or more keys
func (c *Connection) Del(ctx context.Context, keys ...string) (int64, error) {
	result, err := c.Client.Del(ctx, keys...).Result()
	if err != nil {
		return 0, errors.Wrap(err, "Redis del operation failed")
	}
	return result, nil
}

// Exists checks if keys exist
func (c *Connection) Exists(ctx context.Context, keys ...string) (int64, error) {
	result, err := c.Client.Exists(ctx, keys...).Result()
	if err != nil {
		return 0, errors.Wrap(err, "Redis exists operation failed")
	}
	return result, nil
}

// Expire sets expiration for a key
func (c *Connection) Expire(ctx context.Context, key string, expiration time.Duration) error {
	err := c.Client.Expire(ctx, key, expiration).Err()
	if err != nil {
		return errors.Wrap(err, "Redis expire operation failed")
	}
	return nil
}

// TTL returns the time to live for a key
func (c *Connection) TTL(ctx context.Context, key string) (time.Duration, error) {
	result, err := c.Client.TTL(ctx, key).Result()
	if err != nil {
		return 0, errors.Wrap(err, "Redis TTL operation failed")
	}
	return result, nil
}

// Hash operations

// HSet sets fields in a hash
func (c *Connection) HSet(ctx context.Context, key string, values ...interface{}) error {
	err := c.Client.HSet(ctx, key, values...).Err()
	if err != nil {
		return errors.Wrap(err, "Redis HSet operation failed")
	}
	return nil
}

// HGet gets a field from a hash
func (c *Connection) HGet(ctx context.Context, key, field string) (string, error) {
	result, err := c.Client.HGet(ctx, key, field).Result()
	if err == redis.Nil {
		return "", errors.NewNotFound("field not found")
	}
	if err != nil {
		return "", errors.Wrap(err, "Redis HGet operation failed")
	}
	return result, nil
}

// HGetAll gets all fields from a hash
func (c *Connection) HGetAll(ctx context.Context, key string) (map[string]string, error) {
	result, err := c.Client.HGetAll(ctx, key).Result()
	if err != nil {
		return nil, errors.Wrap(err, "Redis HGetAll operation failed")
	}
	return result, nil
}

// HDel deletes fields from a hash
func (c *Connection) HDel(ctx context.Context, key string, fields ...string) (int64, error) {
	result, err := c.Client.HDel(ctx, key, fields...).Result()
	if err != nil {
		return 0, errors.Wrap(err, "Redis HDel operation failed")
	}
	return result, nil
}

// List operations

// LPush prepends values to a list
func (c *Connection) LPush(ctx context.Context, key string, values ...interface{}) error {
	err := c.Client.LPush(ctx, key, values...).Err()
	if err != nil {
		return errors.Wrap(err, "Redis LPush operation failed")
	}
	return nil
}

// RPush appends values to a list
func (c *Connection) RPush(ctx context.Context, key string, values ...interface{}) error {
	err := c.Client.RPush(ctx, key, values...).Err()
	if err != nil {
		return errors.Wrap(err, "Redis RPush operation failed")
	}
	return nil
}

// LRange gets a range of elements from a list
func (c *Connection) LRange(ctx context.Context, key string, start, stop int64) ([]string, error) {
	result, err := c.Client.LRange(ctx, key, start, stop).Result()
	if err != nil {
		return nil, errors.Wrap(err, "Redis LRange operation failed")
	}
	return result, nil
}

// Pipeline operations

// Pipeline creates a new pipeline
func (c *Connection) Pipeline() redis.Pipeliner {
	return c.Client.Pipeline()
}

// TxPipeline creates a new transaction pipeline
func (c *Connection) TxPipeline() redis.Pipeliner {
	return c.Client.TxPipeline()
}

// Lock implements distributed locking

// Lock acquires a distributed lock
func (c *Connection) Lock(ctx context.Context, key string, value string, expiration time.Duration) (bool, error) {
	result, err := c.Client.SetNX(ctx, key, value, expiration).Result()
	if err != nil {
		return false, errors.Wrap(err, "Redis lock operation failed")
	}
	return result, nil
}

// Unlock releases a distributed lock
func (c *Connection) Unlock(ctx context.Context, key string, value string) error {
	// Lua script to ensure we only delete the lock if it has our value
	script := `
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("del", KEYS[1])
		else
			return 0
		end
	`
	
	_, err := c.Client.Eval(ctx, script, []string{key}, value).Result()
	if err != nil {
		return errors.Wrap(err, "Redis unlock operation failed")
	}
	return nil
}

// Subscription operations

// Subscribe subscribes to channels
func (c *Connection) Subscribe(ctx context.Context, channels ...string) *redis.PubSub {
	return c.Client.Subscribe(ctx, channels...)
}

// Publish publishes a message to a channel
func (c *Connection) Publish(ctx context.Context, channel string, message interface{}) error {
	err := c.Client.Publish(ctx, channel, message).Err()
	if err != nil {
		return errors.Wrap(err, "Redis publish operation failed")
	}
	return nil
}

// parseRedisInfo parses Redis INFO command output
func parseRedisInfo(info string) map[string]interface{} {
	result := make(map[string]interface{})
	
	lines := strings.Split(info, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if len(line) > 0 && line[0] != '#' {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				result[parts[0]] = parts[1]
			}
		}
	}
	
	return result
}

// Helper functions

// WithRetry executes a function with retry logic
func (c *Connection) WithRetry(ctx context.Context, maxRetries int, fn func() error) error {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		// Don't retry certain errors
		if errors.IsValidation(err) || errors.IsNotFound(err) {
			return err
		}

		if attempt < maxRetries {
			backoff := time.Duration(attempt+1) * 100 * time.Millisecond
			c.logger.Warn(ctx, "Redis operation failed, retrying", map[string]interface{}{
				"attempt": attempt + 1,
				"backoff": backoff,
				"error":   err.Error(),
			})

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(backoff):
				continue
			}
		}
	}

	return errors.Wrap(lastErr, fmt.Sprintf("Redis operation failed after %d attempts", maxRetries+1))
}