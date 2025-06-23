package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/amiosamu/rocket-science/services/order-service/internal/domain"
	"github.com/amiosamu/rocket-science/services/order-service/internal/repository/interfaces"
	"github.com/amiosamu/rocket-science/shared/platform/errors"
	"github.com/amiosamu/rocket-science/shared/platform/observability/logging"
	"github.com/amiosamu/rocket-science/shared/platform/observability/metrics"
)

// ExternalServices contains all external service dependencies
type ExternalServices struct {
	InventoryClient InventoryClient
	PaymentClient   PaymentClient
	MessageProducer MessageProducer
}

// InventoryClient defines the interface for inventory service communication
type InventoryClient interface {
	CheckAvailability(ctx context.Context, items []domain.CreateOrderItemRequest) ([]InventoryItem, error)
	ReserveItems(ctx context.Context, orderID uuid.UUID, items []domain.CreateOrderItemRequest) error
	ReleaseReservation(ctx context.Context, orderID uuid.UUID) error
}

// PaymentClient defines the interface for payment service communication
type PaymentClient interface {
	ProcessPayment(ctx context.Context, orderID uuid.UUID, amount float64, currency string) (*PaymentResult, error)
}

// MessageProducer defines the interface for message publishing to Kafka
type MessageProducer interface {
	PublishPaymentEvent(ctx context.Context, event PaymentEvent) error
}

// InventoryItem represents an item from inventory service
type InventoryItem struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Price     float64 `json:"price"`
	Available int     `json:"available"`
}

// PaymentResult represents the result of a payment operation
type PaymentResult struct {
	TransactionID string    `json:"transaction_id"`
	Status        string    `json:"status"`
	ProcessedAt   time.Time `json:"processed_at"`
}

// PaymentEvent represents a payment event for Kafka
type PaymentEvent struct {
	OrderID       uuid.UUID `json:"order_id"`
	UserID        uuid.UUID `json:"user_id"`
	Amount        float64   `json:"amount"`
	Currency      string    `json:"currency"`
	TransactionID string    `json:"transaction_id"`
	ProcessedAt   time.Time `json:"processed_at"`
	EventType     string    `json:"event_type"`
}

// OrderService handles order business logic and orchestrates all operations
type OrderService struct {
	repo             interfaces.OrderRepository
	externalServices ExternalServices
	logger           logging.Logger
	metrics          metrics.Metrics
	tracer           trace.Tracer
}

// NewOrderService creates a new order service with all dependencies
func NewOrderService(
	repo interfaces.OrderRepository,
	externalServices ExternalServices,
	logger logging.Logger,
	metrics metrics.Metrics,
) *OrderService {
	return &OrderService{
		repo:             repo,
		externalServices: externalServices,
		logger:           logger,
		metrics:          metrics,
		tracer:           otel.Tracer("order-service"),
	}
}

// CreateOrder creates a new order with full workflow: inventory check → payment → events
func (s *OrderService) CreateOrder(ctx context.Context, req domain.CreateOrderRequest) (*domain.Order, error) {
	ctx, span := s.tracer.Start(ctx, "OrderService.CreateOrder")
	defer span.End()

	span.SetAttributes(
		attribute.String("user_id", req.UserID.String()),
		attribute.Int("items_count", len(req.Items)),
	)

	s.logger.Info(ctx, "Starting order creation", map[string]interface{}{
		"user_id":     req.UserID,
		"items_count": len(req.Items),
	})

	// Step 1: Validate request
	if err := s.validateCreateOrderRequest(req); err != nil {
		span.RecordError(err)
		return nil, errors.Wrap(err, "invalid create order request")
	}

	// Step 2: Check inventory availability
	inventoryItems, err := s.externalServices.InventoryClient.CheckAvailability(ctx, req.Items)
	if err != nil {
		span.RecordError(err)
		s.logger.Error(ctx, "Failed to check inventory availability", err)
		return nil, errors.Wrap(err, "failed to check inventory availability")
	}

	// Step 3: Build order with calculated totals
	order, err := s.buildOrderFromRequest(req, inventoryItems)
	if err != nil {
		span.RecordError(err)
		return nil, errors.Wrap(err, "failed to build order")
	}

	// Step 4: Reserve inventory items
	if err := s.externalServices.InventoryClient.ReserveItems(ctx, order.ID, req.Items); err != nil {
		span.RecordError(err)
		s.logger.Error(ctx, "Failed to reserve inventory items", err)
		return nil, errors.Wrap(err, "failed to reserve inventory items")
	}

	// Step 5: Save order to database
	if err := s.repo.Create(ctx, order); err != nil {
		span.RecordError(err)
		s.logger.Error(ctx, "Failed to create order in database", err)
		// Release inventory reservation on database failure
		s.releaseInventoryReservation(ctx, order.ID)
		return nil, errors.Wrap(err, "failed to create order")
	}

	// Step 6: Process payment
	paymentResult, err := s.processPaymentWithRetry(ctx, order)
	if err != nil {
		span.RecordError(err)
		s.logger.Error(ctx, "Failed to process payment", err)
		// Update order status to failed and release reservation
		s.handlePaymentFailure(ctx, order.ID)
		return nil, errors.Wrap(err, "payment processing failed")
	}

	// Step 7: Update order status to paid
	if err := s.updateOrderStatus(ctx, order.ID, domain.StatusPaid); err != nil {
		s.logger.Error(ctx, "Failed to update order status to paid", err)
		// Continue execution as payment was successful
	}

	// Step 8: Publish payment event to Kafka
	if err := s.publishPaymentEvent(ctx, order, paymentResult); err != nil {
		s.logger.Error(ctx, "Failed to publish payment event", err)
		// Log error but don't fail the order creation since payment succeeded
	}

	// Step 9: Update metrics
	s.updateOrderCreationMetrics(order)

	// Step 10: Get updated order with new status
	updatedOrder, err := s.repo.GetByID(ctx, order.ID)
	if err != nil {
		s.logger.Error(ctx, "Failed to retrieve updated order", err)
		return order, nil // Return original order if retrieval fails
	}

	s.logger.Info(ctx, "Order created successfully", map[string]interface{}{
		"order_id":       order.ID,
		"user_id":        order.UserID,
		"total_amount":   order.TotalAmount,
		"transaction_id": paymentResult.TransactionID,
		"status":         updatedOrder.Status,
	})

	return updatedOrder, nil
}

// GetOrder retrieves an order by ID
func (s *OrderService) GetOrder(ctx context.Context, id uuid.UUID) (*domain.Order, error) {
	ctx, span := s.tracer.Start(ctx, "OrderService.GetOrder")
	defer span.End()

	span.SetAttributes(attribute.String("order_id", id.String()))

	order, err := s.repo.GetByID(ctx, id)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	s.logger.Debug(ctx, "Order retrieved", map[string]interface{}{
		"order_id": id,
		"status":   order.Status,
	})

	return order, nil
}

// GetUserOrders retrieves orders for a specific user with pagination
func (s *OrderService) GetUserOrders(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.Order, error) {
	ctx, span := s.tracer.Start(ctx, "OrderService.GetUserOrders")
	defer span.End()

	span.SetAttributes(
		attribute.String("user_id", userID.String()),
		attribute.Int("limit", limit),
		attribute.Int("offset", offset),
	)

	orders, err := s.repo.GetByUserID(ctx, userID, limit, offset)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	s.logger.Debug(ctx, "User orders retrieved", map[string]interface{}{
		"user_id":      userID,
		"orders_count": len(orders),
		"limit":        limit,
		"offset":       offset,
	})

	return orders, nil
}

// ListOrders retrieves orders based on filter criteria
func (s *OrderService) ListOrders(ctx context.Context, filter domain.OrderFilter) ([]*domain.Order, error) {
	ctx, span := s.tracer.Start(ctx, "OrderService.ListOrders")
	defer span.End()

	orders, err := s.repo.List(ctx, filter)
	if err != nil {
		span.RecordError(err)
		return nil, err
	}

	return orders, nil
}

// UpdateOrderStatus updates the status of an order with validation
func (s *OrderService) UpdateOrderStatus(ctx context.Context, id uuid.UUID, status domain.OrderStatus) error {
	ctx, span := s.tracer.Start(ctx, "OrderService.UpdateOrderStatus")
	defer span.End()

	span.SetAttributes(
		attribute.String("order_id", id.String()),
		attribute.String("status", string(status)),
	)

	// Get current order to validate status transition
	currentOrder, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return err
	}

	// Validate status transition
	if !currentOrder.CanUpdateStatus(status) {
		return errors.NewValidation(fmt.Sprintf("cannot update order status from %s to %s", currentOrder.Status, status))
	}

	return s.updateOrderStatus(ctx, id, status)
}

// HandleAssemblyCompleted handles the assembly completed event from Kafka
func (s *OrderService) HandleAssemblyCompleted(ctx context.Context, orderID uuid.UUID) error {
	ctx, span := s.tracer.Start(ctx, "OrderService.HandleAssemblyCompleted")
	defer span.End()

	span.SetAttributes(attribute.String("order_id", orderID.String()))

	s.logger.Info(ctx, "Processing assembly completed event", map[string]interface{}{
		"order_id": orderID,
	})

	// Update order status to assembled
	if err := s.updateOrderStatus(ctx, orderID, domain.StatusAssembled); err != nil {
		span.RecordError(err)
		s.logger.Error(ctx, "Failed to update order status to assembled", err)
		return err
	}

	// Automatically mark as completed (in real system might have more steps)
	if err := s.updateOrderStatus(ctx, orderID, domain.StatusCompleted); err != nil {
		span.RecordError(err)
		s.logger.Error(ctx, "Failed to update order status to completed", err)
		return err
	}

	// Update completion metrics
	s.metrics.IncrementCounter("orders_completed_total", nil)

	s.logger.Info(ctx, "Order marked as completed", map[string]interface{}{
		"order_id": orderID,
	})

	return nil
}

// GetOrderMetrics returns metrics for monitoring dashboards
func (s *OrderService) GetOrderMetrics(ctx context.Context) (*interfaces.OrderMetrics, error) {
	ctx, span := s.tracer.Start(ctx, "OrderService.GetOrderMetrics")
	defer span.End()

	return s.repo.GetOrderMetrics(ctx)
}

// Private helper methods

func (s *OrderService) validateCreateOrderRequest(req domain.CreateOrderRequest) error {
	if req.UserID == uuid.Nil {
		return errors.NewValidation("user_id is required")
	}

	if len(req.Items) == 0 {
		return errors.NewValidation("at least one item is required")
	}

	for i, item := range req.Items {
		if item.ItemID == "" {
			return errors.NewValidation(fmt.Sprintf("item_id is required for item %d", i))
		}
		if item.Quantity <= 0 {
			return errors.NewValidation(fmt.Sprintf("quantity must be positive for item %d", i))
		}
	}

	return nil
}

func (s *OrderService) buildOrderFromRequest(req domain.CreateOrderRequest, inventoryItems []InventoryItem) (*domain.Order, error) {
	// Create map for quick inventory lookup
	inventoryMap := make(map[string]InventoryItem)
	for _, item := range inventoryItems {
		inventoryMap[item.ID] = item
	}

	order := &domain.Order{
		ID:        uuid.New(),
		UserID:    req.UserID,
		Status:    domain.StatusPending,
		Currency:  "USD",
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
		Items:     make([]domain.OrderItem, 0, len(req.Items)),
	}

	for _, reqItem := range req.Items {
		inventoryItem, exists := inventoryMap[reqItem.ItemID]
		if !exists {
			return nil, errors.NewValidation(fmt.Sprintf("item %s not found in inventory", reqItem.ItemID))
		}

		if inventoryItem.Available < reqItem.Quantity {
			return nil, errors.NewValidation(fmt.Sprintf("insufficient quantity for item %s (requested: %d, available: %d)", 
				reqItem.ItemID, reqItem.Quantity, inventoryItem.Available))
		}

		total := float64(reqItem.Quantity) * inventoryItem.Price

		orderItem := domain.OrderItem{
			ID:        uuid.New(),
			OrderID:   order.ID,
			ItemID:    reqItem.ItemID,
			ItemName:  inventoryItem.Name,
			Quantity:  reqItem.Quantity,
			UnitPrice: inventoryItem.Price,
			Total:     total,
			CreatedAt: time.Now(),
		}

		order.Items = append(order.Items, orderItem)
	}

	order.CalculateTotal()
	return order, nil
}

func (s *OrderService) processPaymentWithRetry(ctx context.Context, order *domain.Order) (*PaymentResult, error) {
	const maxRetries = 3
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		result, err := s.externalServices.PaymentClient.ProcessPayment(ctx, order.ID, order.TotalAmount, order.Currency)
		if err == nil {
			return result, nil
		}

		lastErr = err
		s.logger.Warn(ctx, "Payment attempt failed", map[string]interface{}{
			"order_id": order.ID,
			"attempt":  attempt,
			"error":    err.Error(),
		})

		if attempt < maxRetries {
			time.Sleep(time.Duration(attempt) * time.Second)
		}
	}

	return nil, fmt.Errorf("payment failed after %d attempts: %w", maxRetries, lastErr)
}

func (s *OrderService) publishPaymentEvent(ctx context.Context, order *domain.Order, paymentResult *PaymentResult) error {
	event := PaymentEvent{
		OrderID:       order.ID,
		UserID:        order.UserID,
		Amount:        order.TotalAmount,
		Currency:      order.Currency,
		TransactionID: paymentResult.TransactionID,
		ProcessedAt:   paymentResult.ProcessedAt,
		EventType:     "payment.processed",
	}

	return s.externalServices.MessageProducer.PublishPaymentEvent(ctx, event)
}

func (s *OrderService) updateOrderStatus(ctx context.Context, id uuid.UUID, status domain.OrderStatus) error {
	if err := s.repo.UpdateStatus(ctx, id, status); err != nil {
		return err
	}

	s.metrics.IncrementCounter("order_status_updates_total", map[string]string{
		"status": string(status),
	})

	s.logger.Info(ctx, "Order status updated", map[string]interface{}{
		"order_id": id,
		"status":   status,
	})

	return nil
}

func (s *OrderService) handlePaymentFailure(ctx context.Context, orderID uuid.UUID) {
	// Update order status to failed
	if err := s.updateOrderStatus(ctx, orderID, domain.StatusFailed); err != nil {
		s.logger.Error(ctx, "Failed to update order status to failed", err)
	}

	// Release inventory reservation
	s.releaseInventoryReservation(ctx, orderID)
}

func (s *OrderService) releaseInventoryReservation(ctx context.Context, orderID uuid.UUID) {
	if err := s.externalServices.InventoryClient.ReleaseReservation(ctx, orderID); err != nil {
		s.logger.Error(ctx, "Failed to release inventory reservation", err)
	}
}

func (s *OrderService) updateOrderCreationMetrics(order *domain.Order) {
	s.metrics.IncrementCounter("orders_created_total", map[string]string{
		"status": string(order.Status),
	})
	s.metrics.RecordValue("orders_total_amount", order.TotalAmount, map[string]string{
		"currency": order.Currency,
	})
}