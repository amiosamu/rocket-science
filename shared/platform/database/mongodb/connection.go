package mongodb

import (
	"context"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"

	"github.com/amiosamu/rocket-science/shared/platform/errors"
	"github.com/amiosamu/rocket-science/shared/platform/observability/logging"
)

// Config holds MongoDB connection configuration
type Config struct {
	URI            string        `json:"uri"`
	Database       string        `json:"database"`
	ConnectTimeout time.Duration `json:"connect_timeout"`
	QueryTimeout   time.Duration `json:"query_timeout"`
	MaxPoolSize    uint64        `json:"max_pool_size"`
	MinPoolSize    uint64        `json:"min_pool_size"`
	MaxIdleTime    time.Duration `json:"max_idle_time"`
}

// DefaultConfig returns a default MongoDB configuration
func DefaultConfig() Config {
	return Config{
		URI:            "mongodb://localhost:27017",
		Database:       "app",
		ConnectTimeout: 30 * time.Second,
		QueryTimeout:   30 * time.Second,
		MaxPoolSize:    100,
		MinPoolSize:    5,
		MaxIdleTime:    5 * time.Minute,
	}
}

// Connection manages a MongoDB database connection
type Connection struct {
	Client   *mongo.Client
	Database *mongo.Database
	config   Config
	logger   logging.Logger
}

// NewConnection creates a new MongoDB connection
func NewConnection(config Config, logger logging.Logger) (*Connection, error) {
	// Create context with timeout for connection
	ctx, cancel := context.WithTimeout(context.Background(), config.ConnectTimeout)
	defer cancel()

	// Configure client options
	clientOpts := options.Client().
		ApplyURI(config.URI).
		SetMaxPoolSize(config.MaxPoolSize).
		SetMinPoolSize(config.MinPoolSize).
		SetMaxConnIdleTime(config.MaxIdleTime).
		SetConnectTimeout(config.ConnectTimeout).
		SetServerSelectionTimeout(config.ConnectTimeout)

	// Connect to MongoDB
	client, err := mongo.Connect(ctx, clientOpts)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to MongoDB")
	}

	// Test connection
	if err := client.Ping(ctx, readpref.Primary()); err != nil {
		client.Disconnect(ctx)
		return nil, errors.Wrap(err, "failed to ping MongoDB")
	}

	database := client.Database(config.Database)

	logger.Info(ctx, "MongoDB connection established", map[string]interface{}{
		"uri":             config.URI,
		"database":        config.Database,
		"max_pool_size":   config.MaxPoolSize,
		"min_pool_size":   config.MinPoolSize,
		"connect_timeout": config.ConnectTimeout,
	})

	return &Connection{
		Client:   client,
		Database: database,
		config:   config,
		logger:   logger,
	}, nil
}

// Close closes the MongoDB connection
func (c *Connection) Close() error {
	if c.Client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err := c.Client.Disconnect(ctx)
		if err != nil {
			c.logger.Error(nil, "Failed to close MongoDB connection", err)
			return err
		}
		c.logger.Info(nil, "MongoDB connection closed")
	}
	return nil
}

// HealthCheck performs a health check on the database
func (c *Connection) HealthCheck(ctx context.Context) error {
	if c.Client == nil {
		return errors.NewInternal("MongoDB client is nil")
	}

	// Check if we can ping the database
	if err := c.Client.Ping(ctx, readpref.Primary()); err != nil {
		return errors.Wrap(err, "MongoDB ping failed")
	}

	return nil
}

// GetStats returns MongoDB connection statistics
func (c *Connection) GetStats(ctx context.Context) map[string]interface{} {
	if c.Client == nil {
		return map[string]interface{}{
			"status": "disconnected",
		}
	}

	// Get database stats
	var stats map[string]interface{}
	result := c.Database.RunCommand(ctx, map[string]interface{}{"dbStats": 1})
	if result.Err() == nil {
		result.Decode(&stats)
	}

	// Add connection info
	stats["status"] = "connected"
	stats["database"] = c.config.Database
	stats["max_pool_size"] = c.config.MaxPoolSize
	stats["min_pool_size"] = c.config.MinPoolSize

	return stats
}

// Collection returns a collection with the given name
func (c *Connection) Collection(name string) *mongo.Collection {
	return c.Database.Collection(name)
}

// WithTransaction executes a function within a MongoDB transaction
func (c *Connection) WithTransaction(ctx context.Context, fn func(sessCtx mongo.SessionContext) error) error {
	session, err := c.Client.StartSession()
	if err != nil {
		return errors.Wrap(err, "failed to start MongoDB session")
	}
	defer session.EndSession(ctx)

	callback := func(sessCtx mongo.SessionContext) (interface{}, error) {
		return nil, fn(sessCtx)
	}

	_, err = session.WithTransaction(ctx, callback)
	if err != nil {
		return errors.Wrap(err, "MongoDB transaction failed")
	}

	return nil
}

// WithTransactionRetry executes a function with transaction retry logic
func (c *Connection) WithTransactionRetry(ctx context.Context, maxRetries int, fn func(sessCtx mongo.SessionContext) error) error {
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		err := c.WithTransaction(ctx, fn)
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if !isRetryableError(err) {
			return err
		}

		// Exponential backoff
		if attempt < maxRetries {
			backoff := time.Duration(attempt+1) * 100 * time.Millisecond
			c.logger.Warn(ctx, "MongoDB transaction failed, retrying", map[string]interface{}{
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

	return errors.Wrap(lastErr, fmt.Sprintf("MongoDB transaction failed after %d attempts", maxRetries+1))
}

// CreateIndexes creates indexes for a collection
func (c *Connection) CreateIndexes(ctx context.Context, collectionName string, indexes []mongo.IndexModel) error {
	if len(indexes) == 0 {
		return nil
	}

	collection := c.Collection(collectionName)
	
	indexNames, err := collection.Indexes().CreateMany(ctx, indexes)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to create indexes for collection %s", collectionName))
	}

	c.logger.Info(ctx, "Created indexes", map[string]interface{}{
		"collection": collectionName,
		"indexes":    indexNames,
		"count":      len(indexNames),
	})

	return nil
}

// DropCollection drops a collection if it exists
func (c *Connection) DropCollection(ctx context.Context, collectionName string) error {
	err := c.Database.Collection(collectionName).Drop(ctx)
	if err != nil {
		return errors.Wrap(err, fmt.Sprintf("failed to drop collection %s", collectionName))
	}

	c.logger.Info(ctx, "Dropped collection", map[string]interface{}{
		"collection": collectionName,
	})

	return nil
}

// ListCollections returns a list of collection names in the database
func (c *Connection) ListCollections(ctx context.Context) ([]string, error) {
	cursor, err := c.Database.ListCollectionNames(ctx, map[string]interface{}{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to list collections")
	}

	return cursor, nil
}

// BulkWrite performs a bulk write operation
func (c *Connection) BulkWrite(ctx context.Context, collectionName string, operations []mongo.WriteModel, opts ...*options.BulkWriteOptions) (*mongo.BulkWriteResult, error) {
	collection := c.Collection(collectionName)
	
	result, err := collection.BulkWrite(ctx, operations, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "bulk write operation failed")
	}

	c.logger.Debug(ctx, "Bulk write completed", map[string]interface{}{
		"collection":      collectionName,
		"inserted_count":  result.InsertedCount,
		"modified_count":  result.ModifiedCount,
		"deleted_count":   result.DeletedCount,
		"upserted_count":  result.UpsertedCount,
		"matched_count":   result.MatchedCount,
	})

	return result, nil
}

// Aggregate performs an aggregation pipeline
func (c *Connection) Aggregate(ctx context.Context, collectionName string, pipeline []interface{}, opts ...*options.AggregateOptions) (*mongo.Cursor, error) {
	collection := c.Collection(collectionName)
	
	cursor, err := collection.Aggregate(ctx, pipeline, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "aggregation failed")
	}

	return cursor, nil
}

// isRetryableError checks if a MongoDB error is retryable
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// MongoDB specific retryable errors
	if mongo.IsTimeout(err) {
		return true
	}

	if mongo.IsNetworkError(err) {
		return true
	}

	// Check for specific error codes
	if cmdErr, ok := err.(mongo.CommandError); ok {
		// Retryable error codes
		retryableCodes := []int32{
			11600, // InterruptedAtShutdown
			11602, // InterruptedDueToReplStateChange
			10107, // NotMaster
			13435, // NotMasterNoSlaveOk
			11602, // InterruptedDueToReplStateChange
			189,   // PrimarySteppedDown
		}

		for _, code := range retryableCodes {
			if cmdErr.Code == code {
				return true
			}
		}
	}

	return false
}

// Helper functions for common operations

// EnsureConnection verifies the connection is alive and reconnects if necessary
func (c *Connection) EnsureConnection(ctx context.Context) error {
	if err := c.HealthCheck(ctx); err != nil {
		c.logger.Warn(ctx, "MongoDB connection lost, attempting to reconnect")
		
		// Try to reconnect
		newConn, err := NewConnection(c.config, c.logger)
		if err != nil {
			return errors.Wrap(err, "failed to reconnect to MongoDB")
		}

		// Replace connection details
		c.Client = newConn.Client
		c.Database = newConn.Database

		c.logger.Info(ctx, "MongoDB connection restored")
	}

	return nil
}