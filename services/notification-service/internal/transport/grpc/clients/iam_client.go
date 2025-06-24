package clients

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	iampb "github.com/amiosamu/rocket-science/services/iam-service/proto/iam"
	"github.com/amiosamu/rocket-science/services/notification-service/internal/config"
	"github.com/amiosamu/rocket-science/shared/platform/observability/logging"
	"github.com/amiosamu/rocket-science/shared/platform/observability/metrics"
)

// IAMClient handles communication with the IAM service
type IAMClient struct {
	config  config.IAMClientConfig
	logger  logging.Logger
	metrics metrics.Metrics
	conn    *grpc.ClientConn
	client  iampb.IAMServiceClient
}

// NewIAMClient creates a new IAM client
func NewIAMClient(cfg config.IAMClientConfig, logger logging.Logger, metrics metrics.Metrics) (*IAMClient, error) {
	var opts []grpc.DialOption
	opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))

	address := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	conn, err := grpc.Dial(address, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to IAM service: %w", err)
	}

	client := iampb.NewIAMServiceClient(conn)

	logger.Info(nil, "IAM client created", map[string]interface{}{
		"host": cfg.Host,
		"port": cfg.Port,
	})

	return &IAMClient{
		config:  cfg,
		logger:  logger,
		metrics: metrics,
		conn:    conn,
		client:  client,
	}, nil
}

// GetUserTelegramChatID retrieves the Telegram chat ID for a user
func (c *IAMClient) GetUserTelegramChatID(ctx context.Context, userID string) (int64, error) {
	startTime := time.Now()
	defer func() {
		c.metrics.RecordDuration("iam_get_chat_id_duration", time.Since(startTime), nil)
	}()

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
		return 0, fmt.Errorf("user Telegram chat ID not found")
	}

	c.metrics.IncrementCounter("iam_get_chat_id_success", nil)

	chatID, err := strconv.ParseInt(resp.ChatId, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid chat ID format: %w", err)
	}

	return chatID, nil
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
	// Simple connection check - try to get connection state
	if c.conn == nil {
		return fmt.Errorf("IAM client connection is nil")
	}

	state := c.conn.GetState()
	if state.String() != "READY" && state.String() != "IDLE" {
		return fmt.Errorf("IAM service connection not ready: %s", state.String())
	}

	c.metrics.IncrementCounter("iam_health_check_success", nil)
	return nil
}

// Close closes the IAM client connection
func (c *IAMClient) Close() error {
	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			c.logger.Error(nil, "Failed to close IAM client connection", err, nil)
			return err
		}
	}
	c.logger.Info(nil, "IAM client closed", nil)
	return nil
}
