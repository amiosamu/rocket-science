package domain

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// InventoryItem represents a rocket part in our inventory
// This is our aggregate root for inventory management
type InventoryItem struct {
	// Identity
	id          string // Unique item identifier
	sku         string // Stock Keeping Unit (e.g., "RKT-ENG-001")
	name        string // Human-readable name (e.g., "Raptor Engine")
	description string // Detailed description

	// Categorization
	category ItemCategory // Engine, Fuel Tank, Navigation, etc.

	// Stock management
	stockLevel    int // Current available stock
	reservedStock int // Currently reserved stock
	totalStock    int // Total stock (available + reserved)
	minStockLevel int // Minimum stock threshold
	maxStockLevel int // Maximum stock capacity

	// Reservations
	reservations map[string]*Reservation // Active reservations by order ID

	// Pricing and specifications
	unitPrice      Money             // Price per unit
	weight         float64           // Weight in kg
	dimensions     Dimensions        // Physical dimensions
	specifications map[string]string // Technical specifications

	// Audit fields
	createdAt time.Time // When item was added to inventory
	updatedAt time.Time // Last update timestamp
	version   int       // Version for optimistic locking

	// Status
	status ItemStatus // Active, Discontinued, OutOfStock
}

// ItemCategory represents different types of rocket parts
type ItemCategory int

const (
	CategoryEngines ItemCategory = iota
	CategoryFuelTanks
	CategoryNavigation
	CategoryStructural
	CategoryElectronics
	CategoryLifeSupport
	CategoryPayload
	CategoryLandingGear
)

// String provides human-readable category names
func (ic ItemCategory) String() string {
	switch ic {
	case CategoryEngines:
		return "engines"
	case CategoryFuelTanks:
		return "fuel_tanks"
	case CategoryNavigation:
		return "navigation"
	case CategoryStructural:
		return "structural"
	case CategoryElectronics:
		return "electronics"
	case CategoryLifeSupport:
		return "life_support"
	case CategoryPayload:
		return "payload"
	case CategoryLandingGear:
		return "landing_gear"
	default:
		return "unknown"
	}
}

// ItemStatus represents the lifecycle state of inventory items
type ItemStatus int

const (
	ItemStatusActive ItemStatus = iota
	ItemStatusDiscontinued
	ItemStatusOutOfStock
	ItemStatusBackordered
	ItemStatusIncoming
)

// String provides human-readable status names
func (is ItemStatus) String() string {
	switch is {
	case ItemStatusActive:
		return "active"
	case ItemStatusDiscontinued:
		return "discontinued"
	case ItemStatusOutOfStock:
		return "out_of_stock"
	case ItemStatusBackordered:
		return "backordered"
	case ItemStatusIncoming:
		return "incoming"
	default:
		return "unknown"
	}
}

// Money represents currency amounts (reused from payment service concept)
type Money struct {
	Amount   float64 // The monetary amount
	Currency string  // Currency code (e.g., "USD")
}

// Dimensions represents physical dimensions of rocket parts
type Dimensions struct {
	Length float64 // Length in meters
	Width  float64 // Width in meters
	Height float64 // Height in meters
}

// Reservation represents a temporary stock allocation for an order
type Reservation struct {
	id         string    // Unique reservation identifier
	orderID    string    // Associated order ID
	itemID     string    // Item being reserved
	quantity   int       // Quantity reserved
	reservedAt time.Time // When reservation was made
	expiresAt  time.Time // When reservation expires
	status     ReservationStatus
}

// Reservation getter methods
func (r *Reservation) ID() string                { return r.id }
func (r *Reservation) OrderID() string           { return r.orderID }
func (r *Reservation) ItemID() string            { return r.itemID }
func (r *Reservation) Quantity() int             { return r.quantity }
func (r *Reservation) ReservedAt() time.Time     { return r.reservedAt }
func (r *Reservation) ExpiresAt() time.Time      { return r.expiresAt }
func (r *Reservation) Status() ReservationStatus { return r.status }

// IsExpired checks if the reservation has expired
func (r *Reservation) IsExpired() bool {
	return time.Now().After(r.expiresAt)
}

// IsActive checks if the reservation is currently active
func (r *Reservation) IsActive() bool {
	return r.status == ReservationStatusActive && !r.IsExpired()
}

// ReservationStatus represents the state of a stock reservation
type ReservationStatus int

const (
	ReservationStatusActive ReservationStatus = iota
	ReservationStatusConfirmed
	ReservationStatusExpired
	ReservationStatusCancelled
)

// String provides human-readable reservation status names
func (rs ReservationStatus) String() string {
	switch rs {
	case ReservationStatusActive:
		return "active"
	case ReservationStatusConfirmed:
		return "confirmed"
	case ReservationStatusExpired:
		return "expired"
	case ReservationStatusCancelled:
		return "cancelled"
	default:
		return "unknown"
	}
}

// Domain Events - these represent important inventory events

// StockUpdatedEvent is raised when stock levels change
type StockUpdatedEvent struct {
	ItemID       string
	SKU          string
	OldStock     int
	NewStock     int
	ChangeReason string
	UpdatedAt    time.Time
}

// LowStockEvent is raised when stock falls below threshold
type LowStockEvent struct {
	ItemID       string
	SKU          string
	CurrentStock int
	MinThreshold int
	AlertedAt    time.Time
}

// ReservationCreatedEvent is raised when stock is reserved
type ReservationCreatedEvent struct {
	ReservationID string
	OrderID       string
	ItemID        string
	Quantity      int
	CreatedAt     time.Time
}

// Constructor functions

// NewInventoryItem creates a new inventory item with validation
func NewInventoryItem(sku, name, description string, category ItemCategory, unitPrice Money) (*InventoryItem, error) {
	// Business rule validation
	if sku == "" {
		return nil, ErrInvalidSKU
	}
	if name == "" {
		return nil, ErrInvalidName
	}
	if unitPrice.Amount < 0 {
		return nil, ErrInvalidPrice
	}

	id := uuid.New().String()
	now := time.Now()

	return &InventoryItem{
		id:             id,
		sku:            sku,
		name:           name,
		description:    description,
		category:       category,
		stockLevel:     0,
		reservedStock:  0,
		totalStock:     0,
		minStockLevel:  0,
		maxStockLevel:  1000, // Default max capacity
		reservations:   make(map[string]*Reservation),
		unitPrice:      unitPrice,
		weight:         0.0,
		dimensions:     Dimensions{},
		specifications: make(map[string]string),
		createdAt:      now,
		updatedAt:      now,
		version:        1,
		status:         ItemStatusActive,
	}, nil
}

// ReconstructInventoryItem recreates an inventory item from persisted data
// This method is used by repositories to restore full state from storage
func ReconstructInventoryItem(
	id, sku, name, description string,
	category ItemCategory,
	stockLevel, reservedStock, totalStock, minStockLevel, maxStockLevel int,
	unitPrice Money,
	weight float64,
	dimensions Dimensions,
	specifications map[string]string,
	createdAt, updatedAt time.Time,
	version int,
	status ItemStatus,
) (*InventoryItem, error) {
	// Basic validation for reconstruction
	if id == "" {
		return nil, ErrInvalidItemID
	}
	if sku == "" {
		return nil, ErrInvalidSKU
	}
	if name == "" {
		return nil, ErrInvalidName
	}

	// Create item with full state restoration
	item := &InventoryItem{
		id:             id,
		sku:            sku,
		name:           name,
		description:    description,
		category:       category,
		stockLevel:     stockLevel,
		reservedStock:  reservedStock,
		totalStock:     totalStock,
		minStockLevel:  minStockLevel,
		maxStockLevel:  maxStockLevel,
		reservations:   make(map[string]*Reservation),
		unitPrice:      unitPrice,
		weight:         weight,
		dimensions:     dimensions,
		specifications: specifications,
		createdAt:      createdAt,
		updatedAt:      updatedAt,
		version:        version,
		status:         status,
	}

	// Validate reconstructed state
	if err := item.validateState(); err != nil {
		return nil, fmt.Errorf("invalid reconstructed state: %w", err)
	}

	return item, nil
}

// RestoreReservation restores a reservation during reconstruction
// This method should only be called during object restoration from persistence
func (item *InventoryItem) RestoreReservation(
	id, orderID string,
	quantity int,
	reservedAt, expiresAt time.Time,
	status ReservationStatus,
) error {
	if id == "" {
		return ErrInvalidReservationID
	}
	if orderID == "" {
		return ErrInvalidOrderID
	}
	if quantity <= 0 {
		return ErrInvalidQuantity
	}

	reservation := &Reservation{
		id:         id,
		orderID:    orderID,
		itemID:     item.id,
		quantity:   quantity,
		reservedAt: reservedAt,
		expiresAt:  expiresAt,
		status:     status,
	}

	item.reservations[orderID] = reservation
	return nil
}

// SetInternalState allows setting internal state during reconstruction
// This method should only be used by repositories during object restoration
func (item *InventoryItem) SetInternalState(
	weight float64,
	dimensions Dimensions,
	specifications map[string]string,
	minStockLevel, maxStockLevel int,
) error {
	if minStockLevel < 0 {
		return ErrInvalidStockLevel
	}
	if maxStockLevel < minStockLevel {
		return ErrInvalidStockLevel
	}

	item.weight = weight
	item.dimensions = dimensions
	item.specifications = specifications
	item.minStockLevel = minStockLevel
	item.maxStockLevel = maxStockLevel

	return nil
}

// validateState validates the internal state of a reconstructed item
func (item *InventoryItem) validateState() error {
	// Validate stock levels are consistent
	if item.stockLevel < 0 {
		return fmt.Errorf("stock level cannot be negative")
	}
	if item.reservedStock < 0 {
		return fmt.Errorf("reserved stock cannot be negative")
	}
	if item.totalStock < 0 {
		return fmt.Errorf("total stock cannot be negative")
	}
	if item.stockLevel+item.reservedStock > item.totalStock {
		return fmt.Errorf("available + reserved stock cannot exceed total stock")
	}

	// Validate min/max stock levels
	if item.minStockLevel < 0 {
		return fmt.Errorf("minimum stock level cannot be negative")
	}
	if item.maxStockLevel < item.minStockLevel {
		return fmt.Errorf("maximum stock level cannot be less than minimum")
	}

	// Validate price
	if item.unitPrice.Amount < 0 {
		return fmt.Errorf("unit price cannot be negative")
	}

	return nil
}

// Business methods - these encapsulate inventory business logic

// AddStock increases the available stock
func (item *InventoryItem) AddStock(quantity int, reason string) error {
	if quantity <= 0 {
		return ErrInvalidQuantity
	}

	// oldStock := item.stockLevel // Can be used for event sourcing later
	item.stockLevel += quantity
	item.totalStock += quantity
	item.updatedAt = time.Now()
	item.version++

	// Update status based on new stock level
	item.updateStatus()

	// Could emit domain event here (for future event sourcing)
	// event := StockUpdatedEvent{
	//     ItemID:       item.id,
	//     SKU:          item.sku,
	//     OldStock:     oldStock,
	//     NewStock:     item.stockLevel,
	//     ChangeReason: reason,
	//     UpdatedAt:    item.updatedAt,
	// }

	return nil
}

// RemoveStock decreases the available stock
func (item *InventoryItem) RemoveStock(quantity int, reason string) error {
	if quantity <= 0 {
		return ErrInvalidQuantity
	}
	if quantity > item.stockLevel {
		return ErrInsufficientStock
	}

	// oldStock := item.stockLevel // Can be used for event sourcing later
	item.stockLevel -= quantity
	item.totalStock -= quantity
	item.updatedAt = time.Now()
	item.version++

	// Update status based on new stock level
	item.updateStatus()

	// Could emit domain event here (for future event sourcing)
	// event := StockUpdatedEvent{
	//     ItemID:       item.id,
	//     SKU:          item.sku,
	//     OldStock:     oldStock,
	//     NewStock:     item.stockLevel,
	//     ChangeReason: reason,
	//     UpdatedAt:    item.updatedAt,
	// }

	return nil
}

// ReserveStock creates a reservation for the specified quantity
func (item *InventoryItem) ReserveStock(orderID string, quantity int, expirationMinutes int) (*Reservation, error) {
	// Business rules validation
	if orderID == "" {
		return nil, ErrInvalidOrderID
	}
	if quantity <= 0 {
		return nil, ErrInvalidQuantity
	}
	if quantity > item.GetAvailableStock() {
		return nil, ErrInsufficientStock
	}

	// Check if order already has a reservation
	if _, exists := item.reservations[orderID]; exists {
		return nil, ErrReservationAlreadyExists
	}

	// Create reservation
	reservation := &Reservation{
		id:         uuid.New().String(),
		orderID:    orderID,
		itemID:     item.id,
		quantity:   quantity,
		reservedAt: time.Now(),
		expiresAt:  time.Now().Add(time.Duration(expirationMinutes) * time.Minute),
		status:     ReservationStatusActive,
	}

	// Update stock levels
	item.stockLevel -= quantity
	item.reservedStock += quantity
	item.reservations[orderID] = reservation
	item.updatedAt = time.Now()
	item.version++

	return reservation, nil
}

// ConfirmReservation converts a reservation to a confirmed sale
func (item *InventoryItem) ConfirmReservation(orderID string) error {
	reservation, exists := item.reservations[orderID]
	if !exists {
		return ErrReservationNotFound
	}

	if reservation.status != ReservationStatusActive {
		return ErrInvalidReservationStatus
	}

	// Confirm the reservation (stock already removed from available)
	reservation.status = ReservationStatusConfirmed
	item.reservedStock -= reservation.quantity
	item.totalStock -= reservation.quantity
	item.updatedAt = time.Now()
	item.version++

	// Remove reservation from active reservations
	delete(item.reservations, orderID)

	// Update status
	item.updateStatus()

	return nil
}

// ReleaseReservation cancels a reservation and returns stock to available
func (item *InventoryItem) ReleaseReservation(orderID string) error {
	reservation, exists := item.reservations[orderID]
	if !exists {
		return ErrReservationNotFound
	}

	if reservation.status != ReservationStatusActive {
		return ErrInvalidReservationStatus
	}

	// Return stock to available
	item.stockLevel += reservation.quantity
	item.reservedStock -= reservation.quantity
	reservation.status = ReservationStatusCancelled
	item.updatedAt = time.Now()
	item.version++

	// Remove reservation
	delete(item.reservations, orderID)

	// Update status
	item.updateStatus()

	return nil
}

// CheckAvailability verifies if the requested quantity is available
func (item *InventoryItem) CheckAvailability(quantity int) bool {
	return quantity > 0 && quantity <= item.GetAvailableStock()
}

// CleanupExpiredReservations removes expired reservations and returns stock
func (item *InventoryItem) CleanupExpiredReservations() []string {
	now := time.Now()
	expiredOrders := make([]string, 0)

	for orderID, reservation := range item.reservations {
		if now.After(reservation.expiresAt) {
			// Return stock to available
			item.stockLevel += reservation.quantity
			item.reservedStock -= reservation.quantity
			reservation.status = ReservationStatusExpired

			expiredOrders = append(expiredOrders, orderID)
			delete(item.reservations, orderID)
		}
	}

	if len(expiredOrders) > 0 {
		item.updatedAt = time.Now()
		item.version++
		item.updateStatus()
	}

	return expiredOrders
}

// updateStatus updates item status based on current stock levels
func (item *InventoryItem) updateStatus() {
	if item.status == ItemStatusDiscontinued {
		return // Don't change discontinued items
	}

	if item.stockLevel <= 0 {
		item.status = ItemStatusOutOfStock
	} else if item.stockLevel <= item.minStockLevel {
		// Could trigger low stock alert
		item.status = ItemStatusActive // But still active
	} else {
		item.status = ItemStatusActive
	}
}

// Getter methods

func (item *InventoryItem) ID() string                        { return item.id }
func (item *InventoryItem) SKU() string                       { return item.sku }
func (item *InventoryItem) Name() string                      { return item.name }
func (item *InventoryItem) Description() string               { return item.description }
func (item *InventoryItem) Category() ItemCategory            { return item.category }
func (item *InventoryItem) StockLevel() int                   { return item.stockLevel }
func (item *InventoryItem) ReservedStock() int                { return item.reservedStock }
func (item *InventoryItem) TotalStock() int                   { return item.totalStock }
func (item *InventoryItem) MinStockLevel() int                { return item.minStockLevel }
func (item *InventoryItem) MaxStockLevel() int                { return item.maxStockLevel }
func (item *InventoryItem) UnitPrice() Money                  { return item.unitPrice }
func (item *InventoryItem) Weight() float64                   { return item.weight }
func (item *InventoryItem) Dimensions() Dimensions            { return item.dimensions }
func (item *InventoryItem) Specifications() map[string]string { return item.specifications }
func (item *InventoryItem) CreatedAt() time.Time              { return item.createdAt }
func (item *InventoryItem) UpdatedAt() time.Time              { return item.updatedAt }
func (item *InventoryItem) Version() int                      { return item.version }
func (item *InventoryItem) Status() ItemStatus                { return item.status }

// GetAvailableStock returns stock available for new reservations
func (item *InventoryItem) GetAvailableStock() int {
	return item.stockLevel
}

// IsLowStock checks if item is below minimum threshold
func (item *InventoryItem) IsLowStock() bool {
	return item.stockLevel <= item.minStockLevel
}

// IsOutOfStock checks if item has no available stock
func (item *InventoryItem) IsOutOfStock() bool {
	return item.stockLevel <= 0
}

// GetActiveReservations returns all active reservations
func (item *InventoryItem) GetActiveReservations() []*Reservation {
	reservations := make([]*Reservation, 0, len(item.reservations))
	for _, reservation := range item.reservations {
		if reservation.status == ReservationStatusActive {
			reservations = append(reservations, reservation)
		}
	}
	return reservations
}

// Domain Errors

var (
	ErrInvalidSKU               = errors.New("SKU cannot be empty")
	ErrInvalidName              = errors.New("name cannot be empty")
	ErrInvalidPrice             = errors.New("price cannot be negative")
	ErrInvalidQuantity          = errors.New("quantity must be positive")
	ErrInvalidOrderID           = errors.New("order ID cannot be empty")
	ErrInvalidItemID            = errors.New("item ID cannot be empty")
	ErrInvalidReservationID     = errors.New("reservation ID cannot be empty")
	ErrInvalidStockLevel        = errors.New("invalid stock level")
	ErrInsufficientStock        = errors.New("insufficient stock available")
	ErrReservationNotFound      = errors.New("reservation not found")
	ErrReservationAlreadyExists = errors.New("reservation already exists for this order")
	ErrInvalidReservationStatus = errors.New("invalid reservation status for this operation")
	ErrItemNotFound             = errors.New("inventory item not found")
	ErrItemAlreadyExists        = errors.New("inventory item with this SKU already exists")
)

// Repository interface

// InventoryRepository defines the contract for inventory persistence
type InventoryRepository interface {
	// Save persists an inventory item
	Save(item *InventoryItem) error

	// FindByID retrieves an item by its unique identifier
	FindByID(id string) (*InventoryItem, error)

	// FindBySKU retrieves an item by its SKU
	FindBySKU(sku string) (*InventoryItem, error)

	// FindByCategory retrieves items by category
	FindByCategory(category ItemCategory) ([]*InventoryItem, error)

	// FindLowStockItems retrieves items below minimum threshold
	FindLowStockItems() ([]*InventoryItem, error)

	// FindAvailableItems retrieves items with available stock
	FindAvailableItems() ([]*InventoryItem, error)

	// Delete removes an item from inventory
	Delete(id string) error

	// Search finds items by name or description
	Search(query string) ([]*InventoryItem, error)
}
