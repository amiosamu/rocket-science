package clients

import (
	"context"
	"time"

	// iampb "github.com/amiosamu/rocket-science/services/iam-service/proto/iam"
	"github.com/amiosamu/rocket-science/services/notification-service/internal/config"
	"github.com/amiosamu/rocket-science/shared/platform/observability/logging"
	"github.com/amiosamu/rocket-science/shared/platform/observability/metrics"
)

// Mock types until proto files are generated
type GetUserTelegramChatIDRequest struct {
	UserId string
}

type GetUserTelegramChatIDResponse struct {
	ChatId int64
	Found  bool
}

type UpdateUserTelegramChatIDRequest struct {
	UserId string
	ChatId int64
}

type UpdateUserTelegramChatIDResponse struct {
	Success bool
}

// IAMClient handles communication with the IAM service
type IAMClient struct {
	config  config.IAMClientConfig
	logger  logging.Logger
	metrics metrics.Metrics
	// conn    *grpc.ClientConn // Will be uncommented when proto is available
	// client  iampb.IAMServiceClient // Will be uncommented when proto is available
}

// NewIAMClient creates a new IAM client
func NewIAMClient(cfg config.IAMClientConfig, logger logging.Logger, metrics metrics.Metrics) (*IAMClient, error) {
	// TODO: Implement actual gRPC connection when proto files are available
	/*
		var opts []grpc.DialOption
		if cfg.TLS.Enabled {
			creds, err := credentials.NewClientTLSFromFile(cfg.TLS.CertFile, cfg.TLS.ServerName)
			if err != nil {
				return nil, fmt.Errorf("failed to load TLS credentials: %w", err)
			}
			opts = append(opts, grpc.WithTransportCredentials(creds))
		} else {
			opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
		}

		conn, err := grpc.Dial(cfg.Address(), opts...)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to IAM service: %w", err)
		}

		client := iampb.NewIAMServiceClient(conn)
	*/

	logger.Info(nil, "IAM client created (mock mode)", map[string]interface{}{
		"host": cfg.Host,
		"port": cfg.Port,
	})

	return &IAMClient{
		config:  cfg,
		logger:  logger,
		metrics: metrics,
		// conn:    conn,
		// client:  client,
	}, nil
}

// GetUserTelegramChatID retrieves the Telegram chat ID for a user
func (c *IAMClient) GetUserTelegramChatID(ctx context.Context, userID string) (int64, error) {
	startTime := time.Now()

	// Record metrics
	defer func() {
		c.metrics.RecordDuration("iam_get_chat_id_duration", time.Since(startTime), nil)
	}()

	c.logger.Info(ctx, "Getting user Telegram chat ID", map[string]interface{}{
		"user_id": userID,
	})

	// TODO: Replace with actual gRPC call when proto is available
	/*
		req := &iampb.GetUserTelegramChatIDRequest{
			UserId: userID,
		}

		resp, err := c.client.GetUserTelegramChatID(ctx, req)
		if err != nil {
			c.logger.Error(ctx, "Failed to get user Telegram chat ID", err, map[string]interface{}{
				"user_id": userID,
			})
			c.metrics.IncrementCounter("iam_get_chat_id_error", nil)
			return 0, fmt.Errorf("failed to get user Telegram chat ID: %w", err)
		}

		if !resp.Found {
			c.logger.Warn(ctx, "User Telegram chat ID not found", map[string]interface{}{
				"user_id": userID,
			})
			c.metrics.IncrementCounter("iam_chat_id_not_found", nil)
			return 0, fmt.Errorf("user Telegram chat ID not found")
		}

		c.logger.Info(ctx, "Successfully retrieved user Telegram chat ID", map[string]interface{}{
			"user_id": userID,
			"chat_id": resp.ChatId,
		})
		c.metrics.IncrementCounter("iam_get_chat_id_success", nil)

		return resp.ChatId, nil
	*/

	// Mock implementation for now
	c.logger.Warn(ctx, "Using mock IAM client - returning dummy chat ID", map[string]interface{}{
		"user_id": userID,
	})
	c.metrics.IncrementCounter("iam_get_chat_id_mock", nil)

	// Return a mock chat ID (in production, this should be retrieved from IAM service)
	return 123456789, nil
}

// UpdateUserTelegramChatID updates the Telegram chat ID for a user
func (c *IAMClient) UpdateUserTelegramChatID(ctx context.Context, userID string, chatID int64) error {
	startTime := time.Now()

	// Record metrics
	defer func() {
		c.metrics.RecordDuration("iam_update_chat_id_duration", time.Since(startTime), nil)
	}()

	c.logger.Info(ctx, "Updating user Telegram chat ID", map[string]interface{}{
		"user_id": userID,
		"chat_id": chatID,
	})

	// TODO: Replace with actual gRPC call when proto is available
	/*
		req := &iampb.UpdateUserTelegramChatIDRequest{
			UserId: userID,
			ChatId: chatID,
		}

		resp, err := c.client.UpdateUserTelegramChatID(ctx, req)
		if err != nil {
			c.logger.Error(ctx, "Failed to update user Telegram chat ID", err, map[string]interface{}{
				"user_id": userID,
				"chat_id": chatID,
			})
			c.metrics.IncrementCounter("iam_update_chat_id_error", nil)
			return fmt.Errorf("failed to update user Telegram chat ID: %w", err)
		}

		if !resp.Success {
			c.logger.Error(ctx, "IAM service failed to update chat ID", nil, map[string]interface{}{
				"user_id": userID,
				"chat_id": chatID,
			})
			c.metrics.IncrementCounter("iam_update_chat_id_failed", nil)
			return fmt.Errorf("IAM service failed to update chat ID")
		}

		c.logger.Info(ctx, "Successfully updated user Telegram chat ID", map[string]interface{}{
			"user_id": userID,
			"chat_id": chatID,
		})
		c.metrics.IncrementCounter("iam_update_chat_id_success", nil)

		return nil
	*/

	// Mock implementation for now
	c.logger.Warn(ctx, "Using mock IAM client - update not actually performed", map[string]interface{}{
		"user_id": userID,
		"chat_id": chatID,
	})
	c.metrics.IncrementCounter("iam_update_chat_id_mock", nil)

	return nil
}

// HealthCheck checks if the IAM service is reachable
func (c *IAMClient) HealthCheck(ctx context.Context) error {
	// TODO: Implement actual health check when gRPC is available
	/*
		ctx, cancel := context.WithTimeout(ctx, c.config.ConnectionTimeout)
		defer cancel()

		req := &iampb.HealthCheckRequest{}
		_, err := c.client.HealthCheck(ctx, req)
		if err != nil {
			c.logger.Error(ctx, "IAM service health check failed", err, nil)
			c.metrics.IncrementCounter("iam_health_check_error", nil)
			return fmt.Errorf("IAM service health check failed: %w", err)
		}

		c.metrics.IncrementCounter("iam_health_check_success", nil)
		return nil
	*/

	// Mock implementation
	c.logger.Info(ctx, "IAM health check (mock) - always healthy", nil)
	c.metrics.IncrementCounter("iam_health_check_mock", nil)
	return nil
}

// Close closes the IAM client connection
func (c *IAMClient) Close() error {
	// TODO: Close actual connection when gRPC is available
	/*
		if c.conn != nil {
			if err := c.conn.Close(); err != nil {
				c.logger.Error(nil, "Failed to close IAM client connection", err, nil)
				return err
			}
		}
	*/

	c.logger.Info(nil, "IAM client closed (mock mode)", nil)
	return nil
}
