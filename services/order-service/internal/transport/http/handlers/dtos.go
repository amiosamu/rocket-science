package handlers

import (
	"github.com/google/uuid"
	"github.com/amiosamu/rocket-science/services/order-service/internal/domain"
)

// Request DTOs

// CreateOrderRequest represents the HTTP request to create a new order
type CreateOrderRequest struct {
	UserID uuid.UUID                  `json:"user_id" validate:"required"`
	Items  []CreateOrderItemRequest   `json:"items" validate:"required,min=1"`
}

// CreateOrderItemRequest represents an item in the create order request
type CreateOrderItemRequest struct {
	ItemID   string `json:"item_id" validate:"required"`
	Quantity int    `json:"quantity" validate:"required,min=1"`
}

// UpdateOrderStatusRequest represents the request to update order status
type UpdateOrderStatusRequest struct {
	Status domain.OrderStatus `json:"status" validate:"required"`
}

// Response DTOs

// OrderResponse represents an order in HTTP responses
type OrderResponse struct {
	ID          uuid.UUID           `json:"id"`
	UserID      uuid.UUID           `json:"user_id"`
	Status      string              `json:"status"`
	Items       []OrderItemResponse `json:"items"`
	TotalAmount float64             `json:"total_amount"`
	Currency    string              `json:"currency"`
	CreatedAt   string              `json:"created_at"`
	UpdatedAt   string              `json:"updated_at"`
	PaidAt      *string             `json:"paid_at,omitempty"`
	AssembledAt *string             `json:"assembled_at,omitempty"`
	CompletedAt *string             `json:"completed_at,omitempty"`
}

// OrderItemResponse represents an order item in HTTP responses
type OrderItemResponse struct {
	ID        uuid.UUID `json:"id"`
	ItemID    string    `json:"item_id"`
	ItemName  string    `json:"item_name"`
	Quantity  int       `json:"quantity"`
	UnitPrice float64   `json:"unit_price"`
	Total     float64   `json:"total"`
}

// UserOrdersResponse represents the response for user orders endpoint
type UserOrdersResponse struct {
	Orders     []OrderResponse    `json:"orders"`
	Pagination PaginationResponse `json:"pagination"`
}

// OrderListResponse represents the response for orders list endpoint
type OrderListResponse struct {
	Orders []OrderResponse `json:"orders"`
	Filter FilterResponse  `json:"filter"`
}

// PaginationResponse represents pagination information
type PaginationResponse struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
	Total  int `json:"total"`
}

// FilterResponse represents applied filters
type FilterResponse struct {
	UserID *uuid.UUID           `json:"user_id,omitempty"`
	Status *domain.OrderStatus  `json:"status,omitempty"`
	Limit  int                  `json:"limit"`
	Offset int                  `json:"offset"`
}

// MetricsResponse represents order metrics
type MetricsResponse struct {
	TotalOrders       int                `json:"total_orders"`
	TotalRevenue      float64            `json:"total_revenue"`
	AverageOrderValue float64            `json:"average_order_value"`
	OrdersByStatus    map[string]int     `json:"orders_by_status"`
	OrdersToday       int                `json:"orders_today"`
	RevenueToday      float64            `json:"revenue_today"`
}

// HealthResponse represents health check response
type HealthResponse struct {
	Status    string `json:"status"`
	Service   string `json:"service"`
	Timestamp string `json:"timestamp"`
	Version   string `json:"version"`
}

// ErrorResponse represents error responses
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    int    `json:"code"`
	Details string `json:"details,omitempty"`
}

// SuccessResponse represents generic success responses
type SuccessResponse struct {
	Success bool        `json:"success"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}