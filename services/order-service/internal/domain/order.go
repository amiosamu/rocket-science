package domain

import (
	"time"
	"github.com/google/uuid"
)

// OrderStatus represents the current status of an order
type OrderStatus string

const (
	StatusPending   OrderStatus = "pending"
	StatusPaid      OrderStatus = "paid"
	StatusAssembled OrderStatus = "assembled"
	StatusCompleted OrderStatus = "completed"
	StatusCancelled OrderStatus = "cancelled"
	StatusFailed    OrderStatus = "failed"
)

// OrderItem represents a single item in an order
type OrderItem struct {
	ID        uuid.UUID `json:"id" db:"id"`
	OrderID   uuid.UUID `json:"order_id" db:"order_id"`
	ItemID    string    `json:"item_id" db:"item_id"` // Reference to inventory item
	ItemName  string    `json:"item_name" db:"item_name"`
	Quantity  int       `json:"quantity" db:"quantity"`
	UnitPrice float64   `json:"unit_price" db:"unit_price"`
	Total     float64   `json:"total" db:"total"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// Order represents a customer order
type Order struct {
	ID          uuid.UUID   `json:"id" db:"id"`
	UserID      uuid.UUID   `json:"user_id" db:"user_id"`
	Status      OrderStatus `json:"status" db:"status"`
	Items       []OrderItem `json:"items,omitempty"`
	TotalAmount float64     `json:"total_amount" db:"total_amount"`
	Currency    string      `json:"currency" db:"currency"`
	CreatedAt   time.Time   `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time   `json:"updated_at" db:"updated_at"`
	PaidAt      *time.Time  `json:"paid_at,omitempty" db:"paid_at"`
	AssembledAt *time.Time  `json:"assembled_at,omitempty" db:"assembled_at"`
	CompletedAt *time.Time  `json:"completed_at,omitempty" db:"completed_at"`
}

// CreateOrderRequest represents the request to create a new order
type CreateOrderRequest struct {
	UserID uuid.UUID                `json:"user_id"`
	Items  []CreateOrderItemRequest `json:"items"`
}

// CreateOrderItemRequest represents an item in the create order request
type CreateOrderItemRequest struct {
	ItemID   string `json:"item_id"`
	Quantity int    `json:"quantity"`
}

// UpdateOrderStatusRequest represents the request to update order status
type UpdateOrderStatusRequest struct {
	Status OrderStatus `json:"status"`
}

// OrderFilter represents filters for querying orders
type OrderFilter struct {
	UserID *uuid.UUID   `json:"user_id,omitempty"`
	Status *OrderStatus `json:"status,omitempty"`
	Limit  int          `json:"limit,omitempty"`
	Offset int          `json:"offset,omitempty"`
}

// CalculateTotal calculates the total amount for the order
func (o *Order) CalculateTotal() {
	total := 0.0
	for _, item := range o.Items {
		total += item.Total
	}
	o.TotalAmount = total
}

// CanUpdateStatus checks if the order status can be updated to the new status
func (o *Order) CanUpdateStatus(newStatus OrderStatus) bool {
	switch o.Status {
	case StatusPending:
		return newStatus == StatusPaid || newStatus == StatusCancelled || newStatus == StatusFailed
	case StatusPaid:
		return newStatus == StatusAssembled || newStatus == StatusCancelled || newStatus == StatusFailed
	case StatusAssembled:
		return newStatus == StatusCompleted || newStatus == StatusFailed
	case StatusCompleted, StatusCancelled, StatusFailed:
		return false // Terminal states
	default:
		return false
	}
}

// UpdateStatus updates the order status and sets appropriate timestamps
func (o *Order) UpdateStatus(newStatus OrderStatus) bool {
	if !o.CanUpdateStatus(newStatus) {
		return false
	}

	now := time.Now()
	o.Status = newStatus
	o.UpdatedAt = now

	switch newStatus {
	case StatusPaid:
		o.PaidAt = &now
	case StatusAssembled:
		o.AssembledAt = &now
	case StatusCompleted:
		o.CompletedAt = &now
	}

	return true
}