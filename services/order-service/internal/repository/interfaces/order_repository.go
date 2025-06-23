package interfaces

import (
	"context"
	"github.com/google/uuid"
	"github.com/amiosamu/rocket-science/services/order-service/internal/domain"
)

// OrderRepository defines the interface for order data access operations
type OrderRepository interface {
	// Create creates a new order with its items in a transaction
	Create(ctx context.Context, order *domain.Order) error
	
	// GetByID retrieves an order by its ID, including all items
	GetByID(ctx context.Context, id uuid.UUID) (*domain.Order, error)
	
	// GetByUserID retrieves orders for a specific user with pagination
	GetByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.Order, error)
	
	// Update updates an existing order (including items if modified)
	Update(ctx context.Context, order *domain.Order) error
	
	// UpdateStatus updates only the status and related timestamps of an order
	UpdateStatus(ctx context.Context, id uuid.UUID, status domain.OrderStatus) error
	
	// List retrieves orders based on filter criteria with pagination
	List(ctx context.Context, filter domain.OrderFilter) ([]*domain.Order, error)
	
	// Count returns the total number of orders matching the filter criteria
	Count(ctx context.Context, filter domain.OrderFilter) (int, error)
	
	// Delete soft deletes an order (sets deleted_at timestamp)
	Delete(ctx context.Context, id uuid.UUID) error
	
	// GetOrderMetrics returns aggregated metrics for monitoring and analytics
	GetOrderMetrics(ctx context.Context) (*OrderMetrics, error)
}

// OrderMetrics contains aggregated data for monitoring and reporting
type OrderMetrics struct {
	TotalOrders       int                `json:"total_orders"`
	TotalRevenue      float64            `json:"total_revenue"`
	OrdersByStatus    map[string]int     `json:"orders_by_status"`
	AverageOrderValue float64            `json:"average_order_value"`
	OrdersToday       int                `json:"orders_today"`
	RevenueToday      float64            `json:"revenue_today"`
}