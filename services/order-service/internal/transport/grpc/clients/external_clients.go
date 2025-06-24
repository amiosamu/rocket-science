package clients

import (
	"context"
	"time"

	"github.com/google/uuid"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	inventorypb "github.com/amiosamu/rocket-science/services/inventory-service/proto/inventory"
	"github.com/amiosamu/rocket-science/services/order-service/internal/domain"
	"github.com/amiosamu/rocket-science/services/order-service/internal/service"
	paymentpb "github.com/amiosamu/rocket-science/services/payment-service/proto/payment"
	"github.com/amiosamu/rocket-science/shared/platform/errors"
	"github.com/amiosamu/rocket-science/shared/platform/observability/logging"
)

// InventoryGRPCClient implements the InventoryClient interface using gRPC
type InventoryGRPCClient struct {
	client     inventorypb.InventoryServiceClient
	conn       *grpc.ClientConn
	timeout    time.Duration
	maxRetries int
	retryDelay time.Duration
	logger     logging.Logger
}

// NewInventoryGRPCClient creates a new inventory gRPC client
func NewInventoryGRPCClient(address string, timeout time.Duration, maxRetries int, retryDelay time.Duration, logger logging.Logger) (*InventoryGRPCClient, error) {
	logger.Info(context.Background(), "Connecting to inventory service", map[string]interface{}{
		"address": address,
		"timeout": timeout,
	})

	// Setup gRPC connection with options (remove WithBlock to prevent hanging)
	conn, err := grpc.Dial(address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		// Remove grpc.WithBlock() and grpc.WithTimeout() to prevent startup hanging
		// Connection will be established lazily when first RPC is made
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to inventory service")
	}

	client := inventorypb.NewInventoryServiceClient(conn)

	return &InventoryGRPCClient{
		client:     client,
		conn:       conn,
		timeout:    timeout,
		maxRetries: maxRetries,
		retryDelay: retryDelay,
		logger:     logger,
	}, nil
}

// CheckAvailability checks if items are available in inventory
func (c *InventoryGRPCClient) CheckAvailability(ctx context.Context, items []domain.CreateOrderItemRequest) ([]service.InventoryItem, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// Convert domain items to gRPC request
	grpcItems := make([]*inventorypb.ItemAvailabilityCheck, 0, len(items))
	for _, item := range items {
		grpcItems = append(grpcItems, &inventorypb.ItemAvailabilityCheck{
			Sku:      item.ItemID,
			Quantity: int32(item.Quantity),
		})
	}

	req := &inventorypb.CheckAvailabilityRequest{
		Items: grpcItems,
	}

	c.logger.Debug(ctx, "Checking inventory availability", map[string]interface{}{
		"items_count": len(items),
		"service":     "inventory",
	})

	// Execute with retry logic
	resp, err := c.executeWithRetry(ctx, func() (*inventorypb.CheckAvailabilityResponse, error) {
		return c.client.CheckAvailability(ctx, req)
	})
	if err != nil {
		c.logger.Error(ctx, "Failed to check inventory availability", err)
		return nil, c.handleGRPCError(err, "check availability")
	}

	// Convert gRPC response to domain objects
	inventoryItems := make([]service.InventoryItem, 0, len(resp.Results))
	for _, result := range resp.Results {
		inventoryItems = append(inventoryItems, service.InventoryItem{
			ID:        result.Sku,
			Name:      result.Name,
			Price:     0.0, // Price not available in response
			Available: int(result.AvailableQuantity),
		})
	}

	c.logger.Debug(ctx, "Inventory availability checked successfully", map[string]interface{}{
		"available_items": len(inventoryItems),
	})

	return inventoryItems, nil
}

// ReserveItems reserves items in inventory for an order
func (c *InventoryGRPCClient) ReserveItems(ctx context.Context, orderID uuid.UUID, items []domain.CreateOrderItemRequest) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	// Convert domain items to gRPC request
	grpcItems := make([]*inventorypb.ItemReservationRequest, 0, len(items))
	for _, item := range items {
		grpcItems = append(grpcItems, &inventorypb.ItemReservationRequest{
			Sku:      item.ItemID,
			Quantity: int32(item.Quantity),
		})
	}

	req := &inventorypb.ReserveItemsRequest{
		OrderId: orderID.String(),
		Items:   grpcItems,
	}

	c.logger.Debug(ctx, "Reserving inventory items", map[string]interface{}{
		"order_id":    orderID,
		"items_count": len(items),
	})

	// Execute with retry logic
	_, err := c.executeReserveWithRetry(ctx, func() (*inventorypb.ReserveItemsResponse, error) {
		return c.client.ReserveItems(ctx, req)
	})
	if err != nil {
		c.logger.Error(ctx, "Failed to reserve inventory items", err)
		return c.handleGRPCError(err, "reserve items")
	}

	c.logger.Info(ctx, "Inventory items reserved successfully", map[string]interface{}{
		"order_id": orderID,
	})

	return nil
}

// ReleaseReservation releases a reservation for an order
func (c *InventoryGRPCClient) ReleaseReservation(ctx context.Context, orderID uuid.UUID) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	req := &inventorypb.ReleaseReservationRequest{
		OrderId: orderID.String(),
	}

	c.logger.Debug(ctx, "Releasing inventory reservation", map[string]interface{}{
		"order_id": orderID,
	})

	// Execute with retry logic
	_, err := c.executeReleaseWithRetry(ctx, func() (*inventorypb.ReleaseReservationResponse, error) {
		return c.client.ReleaseReservation(ctx, req)
	})
	if err != nil {
		c.logger.Error(ctx, "Failed to release inventory reservation", err)
		return c.handleGRPCError(err, "release reservation")
	}

	c.logger.Info(ctx, "Inventory reservation released successfully", map[string]interface{}{
		"order_id": orderID,
	})

	return nil
}

// executeWithRetry executes a function with retry logic for inventory operations
func (c *InventoryGRPCClient) executeWithRetry(ctx context.Context, fn func() (*inventorypb.CheckAvailabilityResponse, error)) (*inventorypb.CheckAvailabilityResponse, error) {
	var lastErr error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(c.retryDelay * time.Duration(attempt)):
				// Continue with retry
			}
		}

		resp, err := fn()
		if err == nil {
			return resp, nil
		}

		lastErr = err

		// Don't retry on certain error types
		if st, ok := status.FromError(err); ok {
			switch st.Code() {
			case codes.InvalidArgument, codes.NotFound, codes.AlreadyExists:
				return nil, err // Don't retry these errors
			}
		}

		c.logger.Warn(ctx, "Inventory service call failed, retrying", map[string]interface{}{
			"attempt": attempt + 1,
			"error":   err.Error(),
		})
	}

	return nil, lastErr
}

// executeReserveWithRetry executes reserve operations with retry logic
func (c *InventoryGRPCClient) executeReserveWithRetry(ctx context.Context, fn func() (*inventorypb.ReserveItemsResponse, error)) (*inventorypb.ReserveItemsResponse, error) {
	var lastErr error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(c.retryDelay * time.Duration(attempt)):
				// Continue with retry
			}
		}

		resp, err := fn()
		if err == nil {
			return resp, nil
		}

		lastErr = err

		// Don't retry on certain error types
		if st, ok := status.FromError(err); ok {
			switch st.Code() {
			case codes.InvalidArgument, codes.NotFound, codes.AlreadyExists:
				return nil, err // Don't retry these errors
			}
		}

		c.logger.Warn(ctx, "Inventory service call failed, retrying", map[string]interface{}{
			"attempt": attempt + 1,
			"error":   err.Error(),
		})
	}

	return nil, lastErr
}

// executeReleaseWithRetry executes release operations with retry logic
func (c *InventoryGRPCClient) executeReleaseWithRetry(ctx context.Context, fn func() (*inventorypb.ReleaseReservationResponse, error)) (*inventorypb.ReleaseReservationResponse, error) {
	var lastErr error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(c.retryDelay * time.Duration(attempt)):
				// Continue with retry
			}
		}

		resp, err := fn()
		if err == nil {
			return resp, nil
		}

		lastErr = err

		// Don't retry on certain error types
		if st, ok := status.FromError(err); ok {
			switch st.Code() {
			case codes.InvalidArgument, codes.NotFound, codes.AlreadyExists:
				return nil, err // Don't retry these errors
			}
		}

		c.logger.Warn(ctx, "Inventory service call failed, retrying", map[string]interface{}{
			"attempt": attempt + 1,
			"error":   err.Error(),
		})
	}

	return nil, lastErr
}

// PaymentGRPCClient implements the PaymentClient interface using gRPC
type PaymentGRPCClient struct {
	client     paymentpb.PaymentServiceClient
	conn       *grpc.ClientConn
	timeout    time.Duration
	maxRetries int
	retryDelay time.Duration
	logger     logging.Logger
}

// NewPaymentGRPCClient creates a new payment gRPC client
func NewPaymentGRPCClient(address string, timeout time.Duration, maxRetries int, retryDelay time.Duration, logger logging.Logger) (*PaymentGRPCClient, error) {
	logger.Info(context.Background(), "Connecting to payment service", map[string]interface{}{
		"address": address,
		"timeout": timeout,
	})

	// Setup gRPC connection with options (remove WithBlock to prevent hanging)
	conn, err := grpc.Dial(address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		// Remove grpc.WithBlock() and grpc.WithTimeout() to prevent startup hanging
		// Connection will be established lazily when first RPC is made
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to payment service")
	}

	client := paymentpb.NewPaymentServiceClient(conn)

	return &PaymentGRPCClient{
		client:     client,
		conn:       conn,
		timeout:    timeout,
		maxRetries: maxRetries,
		retryDelay: retryDelay,
		logger:     logger,
	}, nil
}

// ProcessPayment processes payment for an order
func (c *PaymentGRPCClient) ProcessPayment(ctx context.Context, orderID uuid.UUID, amount float64, currency string) (*service.PaymentResult, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	req := &paymentpb.ProcessPaymentRequest{
		OrderId:  orderID.String(),
		Amount:   amount,
		Currency: currency,
	}

	c.logger.Debug(ctx, "Processing payment", map[string]interface{}{
		"order_id": orderID,
		"amount":   amount,
		"currency": currency,
	})

	// Execute with retry logic
	resp, err := c.executePaymentWithRetry(ctx, func() (*paymentpb.ProcessPaymentResponse, error) {
		return c.client.ProcessPayment(ctx, req)
	})
	if err != nil {
		c.logger.Error(ctx, "Failed to process payment", err)
		return nil, c.handleGRPCError(err, "process payment")
	}

	processedAt := time.Now()
	if resp.ProcessedAt != nil {
		processedAt = resp.ProcessedAt.AsTime()
	}

	result := &service.PaymentResult{
		TransactionID: resp.TransactionId,
		Status:        resp.Status.String(),
		ProcessedAt:   processedAt,
	}

	c.logger.Info(ctx, "Payment processed successfully", map[string]interface{}{
		"order_id":       orderID,
		"transaction_id": result.TransactionID,
		"status":         result.Status,
	})

	return result, nil
}

// executePaymentWithRetry executes payment operations with retry logic
func (c *PaymentGRPCClient) executePaymentWithRetry(ctx context.Context, fn func() (*paymentpb.ProcessPaymentResponse, error)) (*paymentpb.ProcessPaymentResponse, error) {
	var lastErr error

	for attempt := 0; attempt <= c.maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(c.retryDelay * time.Duration(attempt)):
				// Continue with retry
			}
		}

		resp, err := fn()
		if err == nil {
			return resp, nil
		}

		lastErr = err

		// Don't retry on certain error types
		if st, ok := status.FromError(err); ok {
			switch st.Code() {
			case codes.InvalidArgument, codes.NotFound, codes.AlreadyExists:
				return nil, err // Don't retry these errors
			}
		}

		c.logger.Warn(ctx, "Payment service call failed, retrying", map[string]interface{}{
			"attempt": attempt + 1,
			"error":   err.Error(),
		})
	}

	return nil, lastErr
}

// Close closes the gRPC connections
func (c *InventoryGRPCClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *PaymentGRPCClient) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// handleGRPCError converts gRPC errors to domain errors
func (c *InventoryGRPCClient) handleGRPCError(err error, operation string) error {
	if st, ok := status.FromError(err); ok {
		switch st.Code() {
		case codes.NotFound:
			return errors.NewNotFound(st.Message())
		case codes.InvalidArgument:
			return errors.NewValidation(st.Message())
		case codes.FailedPrecondition:
			return errors.NewValidation(st.Message())
		case codes.Unavailable:
			return errors.NewInternal("inventory service unavailable")
		case codes.DeadlineExceeded:
			return errors.NewInternal("inventory service timeout")
		default:
			return errors.NewInternal("inventory service error: " + st.Message())
		}
	}
	return errors.Wrap(err, "inventory service "+operation+" failed")
}

func (c *PaymentGRPCClient) handleGRPCError(err error, operation string) error {
	if st, ok := status.FromError(err); ok {
		switch st.Code() {
		case codes.NotFound:
			return errors.NewNotFound(st.Message())
		case codes.InvalidArgument:
			return errors.NewValidation(st.Message())
		case codes.FailedPrecondition:
			return errors.NewValidation(st.Message())
		case codes.Unavailable:
			return errors.NewInternal("payment service unavailable")
		case codes.DeadlineExceeded:
			return errors.NewInternal("payment service timeout")
		default:
			return errors.NewInternal("payment service error: " + st.Message())
		}
	}
	return errors.Wrap(err, "payment service "+operation+" failed")
}
