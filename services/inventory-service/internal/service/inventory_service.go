package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/amiosamu/rocket-science/services/inventory-service/internal/config"
	"github.com/amiosamu/rocket-science/services/inventory-service/internal/domain"
)

// InventoryService defines the interface for inventory operations
// This interface abstracts the business logic from the transport layer
type InventoryService interface {
	// CheckAvailability verifies if requested items are available in stock
	CheckAvailability(ctx context.Context, req CheckAvailabilityRequest) (*CheckAvailabilityResult, error)

	// ReserveItems creates reservations for items in an order
	ReserveItems(ctx context.Context, req ReserveItemsRequest) (*ReserveItemsResult, error)

	// ConfirmReservation confirms reserved items (after payment success)
	ConfirmReservation(ctx context.Context, req ConfirmReservationRequest) (*ConfirmReservationResult, error)

	// ReleaseReservation releases reserved items (if payment fails)
	ReleaseReservation(ctx context.Context, req ReleaseReservationRequest) (*ReleaseReservationResult, error)

	// GetItem retrieves details of a specific inventory item
	GetItem(ctx context.Context, req GetItemRequest) (*GetItemResult, error)

	// SearchItems searches for items by query, category, or availability
	SearchItems(ctx context.Context, req SearchItemsRequest) (*SearchItemsResult, error)

	// UpdateStock adds or removes stock (admin operation)
	UpdateStock(ctx context.Context, req UpdateStockRequest) (*UpdateStockResult, error)

	// GetLowStockItems retrieves items below minimum stock threshold
	GetLowStockItems(ctx context.Context, req GetLowStockItemsRequest) (*GetLowStockItemsResult, error)

	// GetItemsByCategory retrieves items in a specific category
	GetItemsByCategory(ctx context.Context, req GetItemsByCategoryRequest) (*GetItemsByCategoryResult, error)

	// CleanupExpiredReservations removes expired reservations across all items
	CleanupExpiredReservations(ctx context.Context) (*CleanupResult, error)
}

// Service DTOs - Data Transfer Objects for the service layer

type CheckAvailabilityRequest struct {
	Items []ItemAvailabilityCheck
}

type ItemAvailabilityCheck struct {
	SKU      string
	Quantity int
}

type CheckAvailabilityResult struct {
	AllAvailable bool
	Results      []ItemAvailabilityResult
	Message      string
}

type ItemAvailabilityResult struct {
	SKU               string
	Name              string
	Available         bool
	RequestedQuantity int
	AvailableQuantity int
	ReservedQuantity  int
	Reason            string
}

type ReserveItemsRequest struct {
	OrderID                    string
	Items                      []ItemReservationRequest
	ReservationDurationMinutes int
}

type ItemReservationRequest struct {
	SKU      string
	Quantity int
}

type ReserveItemsResult struct {
	Success       bool
	ReservationID string
	Results       []ItemReservationResult
	ExpiresAt     time.Time
	Message       string
}

type ItemReservationResult struct {
	SKU           string
	Name          string
	Reserved      bool
	Quantity      int
	ReservationID string
	Reason        string
}

type ConfirmReservationRequest struct {
	OrderID       string
	ReservationID string
}

type ConfirmReservationResult struct {
	Success     bool
	Results     []ItemConfirmationResult
	ConfirmedAt time.Time
	Message     string
}

type ItemConfirmationResult struct {
	SKU       string
	Name      string
	Confirmed bool
	Quantity  int
	Reason    string
}

type ReleaseReservationRequest struct {
	OrderID       string
	ReservationID string
	Reason        string
}

type ReleaseReservationResult struct {
	Success    bool
	Results    []ItemReleaseResult
	ReleasedAt time.Time
	Message    string
}

type ItemReleaseResult struct {
	SKU      string
	Name     string
	Released bool
	Quantity int
	Reason   string
}

type GetItemRequest struct {
	ItemID string
	SKU    string
}

type GetItemResult struct {
	Found   bool
	Item    *InventoryItemDTO
	Message string
}

type SearchItemsRequest struct {
	Query         string
	Category      *domain.ItemCategory
	AvailableOnly bool
	Limit         int
	Offset        int
}

type SearchItemsResult struct {
	Items      []InventoryItemDTO
	TotalCount int
	HasMore    bool
	Message    string
}

type UpdateStockRequest struct {
	SKU            string
	QuantityChange int
	Reason         string
	UpdatedBy      string
}

type UpdateStockResult struct {
	Success       bool
	OldStockLevel int
	NewStockLevel int
	UpdatedAt     time.Time
	Message       string
}

type GetLowStockItemsRequest struct {
	Category          *domain.ItemCategory
	ThresholdOverride *int
}

type GetLowStockItemsResult struct {
	Items      []LowStockItemDTO
	TotalCount int
	Message    string
}

type GetItemsByCategoryRequest struct {
	Category      domain.ItemCategory
	AvailableOnly bool
	Limit         int
	Offset        int
}

type GetItemsByCategoryResult struct {
	Items      []InventoryItemDTO
	TotalCount int
	HasMore    bool
	Message    string
}

type CleanupResult struct {
	CleanedReservations int
	AffectedItems       []string
	Message             string
}

// DTOs for complex objects

type InventoryItemDTO struct {
	ID             string
	SKU            string
	Name           string
	Description    string
	Category       domain.ItemCategory
	StockLevel     int
	ReservedStock  int
	TotalStock     int
	MinStockLevel  int
	MaxStockLevel  int
	UnitPrice      domain.Money
	Weight         float64
	Dimensions     domain.Dimensions
	Specifications map[string]string
	CreatedAt      time.Time
	UpdatedAt      time.Time
	Version        int
	Status         domain.ItemStatus
}

type LowStockItemDTO struct {
	Item             InventoryItemDTO
	ShortageQuantity int
	DaysOfStock      int
}

// inventoryService is the concrete implementation of InventoryService
type inventoryService struct {
	config     *config.Config
	logger     *slog.Logger
	repository domain.InventoryRepository
}

// NewInventoryService creates a new inventory service with dependencies
func NewInventoryService(cfg *config.Config, logger *slog.Logger, repository domain.InventoryRepository) InventoryService {
	return &inventoryService{
		config:     cfg,
		logger:     logger,
		repository: repository,
	}
}

// CheckAvailability verifies if requested items are available in stock
func (s *inventoryService) CheckAvailability(ctx context.Context, req CheckAvailabilityRequest) (*CheckAvailabilityResult, error) {
	s.logger.Info("Checking availability for items", "itemCount", len(req.Items))

	results := make([]ItemAvailabilityResult, 0, len(req.Items))
	allAvailable := true

	for _, item := range req.Items {
		// Validate request
		if err := s.validateAvailabilityCheck(item); err != nil {
			result := ItemAvailabilityResult{
				SKU:               item.SKU,
				Available:         false,
				RequestedQuantity: item.Quantity,
				Reason:            fmt.Sprintf("Invalid request: %v", err),
			}
			results = append(results, result)
			allAvailable = false
			continue
		}

		// Find item by SKU
		inventoryItem, err := s.repository.FindBySKU(item.SKU)
		if err != nil {
			s.logger.Error("Failed to find item by SKU", "sku", item.SKU, "error", err)
			result := ItemAvailabilityResult{
				SKU:               item.SKU,
				Available:         false,
				RequestedQuantity: item.Quantity,
				Reason:            "Failed to retrieve item information",
			}
			results = append(results, result)
			allAvailable = false
			continue
		}

		if inventoryItem == nil {
			result := ItemAvailabilityResult{
				SKU:               item.SKU,
				Available:         false,
				RequestedQuantity: item.Quantity,
				Reason:            "Item not found",
			}
			results = append(results, result)
			allAvailable = false
			continue
		}

		// Check availability
		available := inventoryItem.CheckAvailability(item.Quantity)
		reason := ""
		if !available {
			if inventoryItem.IsOutOfStock() {
				reason = "Out of stock"
			} else {
				reason = fmt.Sprintf("Insufficient stock (available: %d, requested: %d)",
					inventoryItem.GetAvailableStock(), item.Quantity)
			}
		}

		result := ItemAvailabilityResult{
			SKU:               inventoryItem.SKU(),
			Name:              inventoryItem.Name(),
			Available:         available,
			RequestedQuantity: item.Quantity,
			AvailableQuantity: inventoryItem.GetAvailableStock(),
			ReservedQuantity:  inventoryItem.ReservedStock(),
			Reason:            reason,
		}
		results = append(results, result)

		if !available {
			allAvailable = false
		}
	}

	message := "All items available"
	if !allAvailable {
		message = "Some items are not available in requested quantities"
	}

	s.logger.Info("Availability check completed",
		"allAvailable", allAvailable,
		"itemsChecked", len(req.Items))

	return &CheckAvailabilityResult{
		AllAvailable: allAvailable,
		Results:      results,
		Message:      message,
	}, nil
}

// ReserveItems creates reservations for items in an order
func (s *inventoryService) ReserveItems(ctx context.Context, req ReserveItemsRequest) (*ReserveItemsResult, error) {
	s.logger.Info("Creating reservations for order",
		"orderID", req.OrderID,
		"itemCount", len(req.Items))

	// Validate request
	if err := s.validateReserveItemsRequest(req); err != nil {
		return &ReserveItemsResult{
			Success: false,
			Message: fmt.Sprintf("Invalid request: %v", err),
		}, nil
	}

	results := make([]ItemReservationResult, 0, len(req.Items))
	allReserved := true
	reservationID := s.generateReservationID(req.OrderID)

	// Calculate expiration time
	expiresAt := time.Now().Add(time.Duration(req.ReservationDurationMinutes) * time.Minute)

	// Process each item reservation
	for _, item := range req.Items {
		result := s.processItemReservation(item, req.OrderID, req.ReservationDurationMinutes)
		results = append(results, result)

		if !result.Reserved {
			allReserved = false
		}
	}

	// If any reservation failed, release all successful reservations
	if !allReserved {
		s.logger.Warn("Some reservations failed, releasing successful ones", "orderID", req.OrderID)
		s.releasePartialReservations(req.OrderID, results)
	}

	message := "All items reserved successfully"
	if !allReserved {
		message = "Some items could not be reserved"
	}

	s.logger.Info("Reservation process completed",
		"orderID", req.OrderID,
		"success", allReserved,
		"reservationID", reservationID)

	return &ReserveItemsResult{
		Success:       allReserved,
		ReservationID: reservationID,
		Results:       results,
		ExpiresAt:     expiresAt,
		Message:       message,
	}, nil
}

// processItemReservation handles reservation for a single item
func (s *inventoryService) processItemReservation(item ItemReservationRequest, orderID string, durationMinutes int) ItemReservationResult {
	// Find item by SKU
	inventoryItem, err := s.repository.FindBySKU(item.SKU)
	if err != nil {
		s.logger.Error("Failed to find item for reservation", "sku", item.SKU, "error", err)
		return ItemReservationResult{
			SKU:      item.SKU,
			Reserved: false,
			Quantity: item.Quantity,
			Reason:   "Failed to retrieve item information",
		}
	}

	if inventoryItem == nil {
		return ItemReservationResult{
			SKU:      item.SKU,
			Reserved: false,
			Quantity: item.Quantity,
			Reason:   "Item not found",
		}
	}

	// Attempt to reserve stock
	reservation, err := inventoryItem.ReserveStock(orderID, item.Quantity, durationMinutes)
	if err != nil {
		reason := err.Error()
		if err == domain.ErrInsufficientStock {
			reason = fmt.Sprintf("Insufficient stock (available: %d, requested: %d)",
				inventoryItem.GetAvailableStock(), item.Quantity)
		}

		return ItemReservationResult{
			SKU:      inventoryItem.SKU(),
			Name:     inventoryItem.Name(),
			Reserved: false,
			Quantity: item.Quantity,
			Reason:   reason,
		}
	}

	// Save updated item
	if err := s.repository.Save(inventoryItem); err != nil {
		s.logger.Error("Failed to save item after reservation",
			"sku", item.SKU, "orderID", orderID, "error", err)

		// Try to release the reservation in memory
		inventoryItem.ReleaseReservation(orderID)

		return ItemReservationResult{
			SKU:      inventoryItem.SKU(),
			Name:     inventoryItem.Name(),
			Reserved: false,
			Quantity: item.Quantity,
			Reason:   "Failed to save reservation",
		}
	}

	s.logger.Debug("Item reserved successfully",
		"sku", item.SKU,
		"orderID", orderID,
		"quantity", item.Quantity,
		"reservationID", reservation.ID())

	return ItemReservationResult{
		SKU:           inventoryItem.SKU(),
		Name:          inventoryItem.Name(),
		Reserved:      true,
		Quantity:      item.Quantity,
		ReservationID: reservation.ID(),
		Reason:        "",
	}
}

// ConfirmReservation confirms reserved items (after payment success)
func (s *inventoryService) ConfirmReservation(ctx context.Context, req ConfirmReservationRequest) (*ConfirmReservationResult, error) {
	s.logger.Info("Confirming reservation",
		"orderID", req.OrderID,
		"reservationID", req.ReservationID)

	// Validate request
	if req.OrderID == "" {
		return &ConfirmReservationResult{
			Success: false,
			Message: "Order ID is required",
		}, nil
	}

	// Find all items with reservations for this order
	// Note: In a real implementation, you might want to track reservations separately
	// For now, we'll search through available items
	availableItems, err := s.repository.FindAvailableItems()
	if err != nil {
		s.logger.Error("Failed to find available items for confirmation", "error", err)
		return nil, fmt.Errorf("failed to find items: %w", err)
	}

	results := make([]ItemConfirmationResult, 0)
	allConfirmed := true
	confirmedAt := time.Now()

	// Process confirmations for items with reservations for this order
	for _, item := range availableItems {
		// Check if this item has a reservation for the order
		reservations := item.GetActiveReservations()
		hasReservation := false

		for _, reservation := range reservations {
			if reservation.OrderID() == req.OrderID {
				hasReservation = true
				break
			}
		}

		if !hasReservation {
			continue // Skip items without reservations for this order
		}

		// Confirm the reservation
		err := item.ConfirmReservation(req.OrderID)
		if err != nil {
			s.logger.Error("Failed to confirm reservation",
				"orderID", req.OrderID,
				"sku", item.SKU(),
				"error", err)

			result := ItemConfirmationResult{
				SKU:       item.SKU(),
				Name:      item.Name(),
				Confirmed: false,
				Reason:    err.Error(),
			}
			results = append(results, result)
			allConfirmed = false
			continue
		}

		// Save updated item
		if err := s.repository.Save(item); err != nil {
			s.logger.Error("Failed to save item after confirmation",
				"sku", item.SKU(),
				"orderID", req.OrderID,
				"error", err)

			result := ItemConfirmationResult{
				SKU:       item.SKU(),
				Name:      item.Name(),
				Confirmed: false,
				Reason:    "Failed to save confirmation",
			}
			results = append(results, result)
			allConfirmed = false
			continue
		}

		result := ItemConfirmationResult{
			SKU:       item.SKU(),
			Name:      item.Name(),
			Confirmed: true,
			Quantity:  0, // We'd need to track the original quantity
			Reason:    "",
		}
		results = append(results, result)
	}

	message := "All reservations confirmed successfully"
	if !allConfirmed {
		message = "Some reservations could not be confirmed"
	}

	s.logger.Info("Reservation confirmation completed",
		"orderID", req.OrderID,
		"success", allConfirmed,
		"itemsConfirmed", len(results))

	return &ConfirmReservationResult{
		Success:     allConfirmed,
		Results:     results,
		ConfirmedAt: confirmedAt,
		Message:     message,
	}, nil
}

// ReleaseReservation releases reserved items (if payment fails)
func (s *inventoryService) ReleaseReservation(ctx context.Context, req ReleaseReservationRequest) (*ReleaseReservationResult, error) {
	s.logger.Info("Releasing reservation",
		"orderID", req.OrderID,
		"reservationID", req.ReservationID,
		"reason", req.Reason)

	// Validate request
	if req.OrderID == "" {
		return &ReleaseReservationResult{
			Success: false,
			Message: "Order ID is required",
		}, nil
	}

	// Find all items with reservations for this order
	availableItems, err := s.repository.FindAvailableItems()
	if err != nil {
		s.logger.Error("Failed to find available items for release", "error", err)
		return nil, fmt.Errorf("failed to find items: %w", err)
	}

	results := make([]ItemReleaseResult, 0)
	allReleased := true
	releasedAt := time.Now()

	// Process releases for items with reservations for this order
	for _, item := range availableItems {
		// Check if this item has a reservation for the order
		reservations := item.GetActiveReservations()
		hasReservation := false
		var reservationQuantity int

		for _, reservation := range reservations {
			if reservation.OrderID() == req.OrderID {
				hasReservation = true
				reservationQuantity = reservation.Quantity()
				break
			}
		}

		if !hasReservation {
			continue // Skip items without reservations for this order
		}

		// Release the reservation
		err := item.ReleaseReservation(req.OrderID)
		if err != nil {
			s.logger.Error("Failed to release reservation",
				"orderID", req.OrderID,
				"sku", item.SKU(),
				"error", err)

			result := ItemReleaseResult{
				SKU:      item.SKU(),
				Name:     item.Name(),
				Released: false,
				Quantity: reservationQuantity,
				Reason:   err.Error(),
			}
			results = append(results, result)
			allReleased = false
			continue
		}

		// Save updated item
		if err := s.repository.Save(item); err != nil {
			s.logger.Error("Failed to save item after release",
				"sku", item.SKU(),
				"orderID", req.OrderID,
				"error", err)

			result := ItemReleaseResult{
				SKU:      item.SKU(),
				Name:     item.Name(),
				Released: false,
				Quantity: reservationQuantity,
				Reason:   "Failed to save release",
			}
			results = append(results, result)
			allReleased = false
			continue
		}

		result := ItemReleaseResult{
			SKU:      item.SKU(),
			Name:     item.Name(),
			Released: true,
			Quantity: reservationQuantity,
			Reason:   "",
		}
		results = append(results, result)
	}

	message := "All reservations released successfully"
	if !allReleased {
		message = "Some reservations could not be released"
	}

	s.logger.Info("Reservation release completed",
		"orderID", req.OrderID,
		"success", allReleased,
		"itemsReleased", len(results))

	return &ReleaseReservationResult{
		Success:    allReleased,
		Results:    results,
		ReleasedAt: releasedAt,
		Message:    message,
	}, nil
}

// GetItem retrieves details of a specific inventory item
func (s *inventoryService) GetItem(ctx context.Context, req GetItemRequest) (*GetItemResult, error) {
	s.logger.Debug("Getting item", "itemID", req.ItemID, "sku", req.SKU)

	var item *domain.InventoryItem
	var err error

	// Find by ID or SKU
	if req.ItemID != "" {
		item, err = s.repository.FindByID(req.ItemID)
	} else if req.SKU != "" {
		item, err = s.repository.FindBySKU(req.SKU)
	} else {
		return &GetItemResult{
			Found:   false,
			Message: "Either item ID or SKU must be provided",
		}, nil
	}

	if err != nil {
		s.logger.Error("Failed to find item", "error", err)
		return nil, fmt.Errorf("failed to find item: %w", err)
	}

	if item == nil {
		return &GetItemResult{
			Found:   false,
			Message: "Item not found",
		}, nil
	}

	// Clean up expired reservations
	expiredOrders := item.CleanupExpiredReservations()
	if len(expiredOrders) > 0 {
		s.logger.Info("Cleaned up expired reservations",
			"sku", item.SKU(),
			"expiredOrders", expiredOrders)
		s.repository.Save(item)
	}

	itemDTO := s.convertDomainToDTO(item)

	return &GetItemResult{
		Found:   true,
		Item:    &itemDTO,
		Message: "Item found successfully",
	}, nil
}

// SearchItems searches for items by query, category, or availability
func (s *inventoryService) SearchItems(ctx context.Context, req SearchItemsRequest) (*SearchItemsResult, error) {
	s.logger.Debug("Searching items",
		"query", req.Query,
		"category", req.Category,
		"availableOnly", req.AvailableOnly)

	var items []*domain.InventoryItem
	var err error

	// Determine search strategy
	if req.Category != nil {
		// Search by category
		items, err = s.repository.FindByCategory(*req.Category)
	} else if req.Query != "" {
		// Text search
		items, err = s.repository.Search(req.Query)
	} else {
		// Get all available items
		items, err = s.repository.FindAvailableItems()
	}

	if err != nil {
		s.logger.Error("Failed to search items", "error", err)
		return nil, fmt.Errorf("failed to search items: %w", err)
	}

	// Filter by availability if requested
	if req.AvailableOnly {
		filteredItems := make([]*domain.InventoryItem, 0)
		for _, item := range items {
			if !item.IsOutOfStock() {
				filteredItems = append(filteredItems, item)
			}
		}
		items = filteredItems
	}

	// Apply pagination
	totalCount := len(items)
	startIdx := req.Offset
	endIdx := req.Offset + req.Limit

	if startIdx >= totalCount {
		items = []*domain.InventoryItem{}
	} else {
		if endIdx > totalCount {
			endIdx = totalCount
		}
		items = items[startIdx:endIdx]
	}

	// Convert to DTOs
	itemDTOs := make([]InventoryItemDTO, len(items))
	for i, item := range items {
		itemDTOs[i] = s.convertDomainToDTO(item)
	}

	hasMore := req.Offset+len(items) < totalCount

	return &SearchItemsResult{
		Items:      itemDTOs,
		TotalCount: totalCount,
		HasMore:    hasMore,
		Message:    fmt.Sprintf("Found %d items", totalCount),
	}, nil
}

// UpdateStock adds or removes stock (admin operation)
func (s *inventoryService) UpdateStock(ctx context.Context, req UpdateStockRequest) (*UpdateStockResult, error) {
	s.logger.Info("Updating stock",
		"sku", req.SKU,
		"quantityChange", req.QuantityChange,
		"reason", req.Reason,
		"updatedBy", req.UpdatedBy)

	// Validate request
	if req.SKU == "" {
		return &UpdateStockResult{
			Success: false,
			Message: "SKU is required",
		}, nil
	}

	// Find item
	item, err := s.repository.FindBySKU(req.SKU)
	if err != nil {
		s.logger.Error("Failed to find item for stock update", "sku", req.SKU, "error", err)
		return nil, fmt.Errorf("failed to find item: %w", err)
	}

	if item == nil {
		return &UpdateStockResult{
			Success: false,
			Message: "Item not found",
		}, nil
	}

	oldStockLevel := item.StockLevel()

	// Apply stock change
	if req.QuantityChange > 0 {
		// Adding stock
		err = item.AddStock(req.QuantityChange, req.Reason)
	} else if req.QuantityChange < 0 {
		// Removing stock
		err = item.RemoveStock(-req.QuantityChange, req.Reason)
	} else {
		// No change
		return &UpdateStockResult{
			Success:       true,
			OldStockLevel: oldStockLevel,
			NewStockLevel: oldStockLevel,
			UpdatedAt:     time.Now(),
			Message:       "No stock change applied",
		}, nil
	}

	if err != nil {
		s.logger.Warn("Failed to update stock",
			"sku", req.SKU,
			"quantityChange", req.QuantityChange,
			"error", err)

		return &UpdateStockResult{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	// Save updated item
	if err := s.repository.Save(item); err != nil {
		s.logger.Error("Failed to save item after stock update",
			"sku", req.SKU,
			"error", err)
		return nil, fmt.Errorf("failed to save item: %w", err)
	}

	newStockLevel := item.StockLevel()
	updatedAt := item.UpdatedAt()

	s.logger.Info("Stock updated successfully",
		"sku", req.SKU,
		"oldStock", oldStockLevel,
		"newStock", newStockLevel,
		"change", req.QuantityChange)

	return &UpdateStockResult{
		Success:       true,
		OldStockLevel: oldStockLevel,
		NewStockLevel: newStockLevel,
		UpdatedAt:     updatedAt,
		Message:       "Stock updated successfully",
	}, nil
}

// GetLowStockItems retrieves items below minimum stock threshold
func (s *inventoryService) GetLowStockItems(ctx context.Context, req GetLowStockItemsRequest) (*GetLowStockItemsResult, error) {
	s.logger.Debug("Getting low stock items", "category", req.Category)

	var items []*domain.InventoryItem
	var err error

	if req.Category != nil {
		// Get items by category first, then filter
		categoryItems, err := s.repository.FindByCategory(*req.Category)
		if err != nil {
			s.logger.Error("Failed to find items by category", "error", err)
			return nil, fmt.Errorf("failed to find items: %w", err)
		}

		// Filter for low stock
		for _, item := range categoryItems {
			if item.IsLowStock() {
				items = append(items, item)
			}
		}
	} else {
		// Get all low stock items
		items, err = s.repository.FindLowStockItems()
		if err != nil {
			s.logger.Error("Failed to find low stock items", "error", err)
			return nil, fmt.Errorf("failed to find low stock items: %w", err)
		}
	}

	// Convert to low stock DTOs
	lowStockItems := make([]LowStockItemDTO, len(items))
	for i, item := range items {
		shortageQuantity := item.MinStockLevel() - item.StockLevel()
		if shortageQuantity < 0 {
			shortageQuantity = 0
		}

		// Simple estimation of days of stock (assuming some average usage)
		daysOfStock := 0
		if item.StockLevel() > 0 {
			// This is a simplified calculation - in reality you'd use historical usage data
			daysOfStock = item.StockLevel() / max(1, item.MinStockLevel()/30) // Assume min stock lasts 30 days
		}

		lowStockItems[i] = LowStockItemDTO{
			Item:             s.convertDomainToDTO(item),
			ShortageQuantity: shortageQuantity,
			DaysOfStock:      daysOfStock,
		}
	}

	return &GetLowStockItemsResult{
		Items:      lowStockItems,
		TotalCount: len(lowStockItems),
		Message:    fmt.Sprintf("Found %d low stock items", len(lowStockItems)),
	}, nil
}

// GetItemsByCategory retrieves items in a specific category
func (s *inventoryService) GetItemsByCategory(ctx context.Context, req GetItemsByCategoryRequest) (*GetItemsByCategoryResult, error) {
	s.logger.Debug("Getting items by category",
		"category", req.Category,
		"availableOnly", req.AvailableOnly)

	items, err := s.repository.FindByCategory(req.Category)
	if err != nil {
		s.logger.Error("Failed to find items by category", "error", err)
		return nil, fmt.Errorf("failed to find items: %w", err)
	}

	// Filter by availability if requested
	if req.AvailableOnly {
		filteredItems := make([]*domain.InventoryItem, 0)
		for _, item := range items {
			if !item.IsOutOfStock() {
				filteredItems = append(filteredItems, item)
			}
		}
		items = filteredItems
	}

	// Apply pagination
	totalCount := len(items)
	startIdx := req.Offset
	endIdx := req.Offset + req.Limit

	if startIdx >= totalCount {
		items = []*domain.InventoryItem{}
	} else {
		if endIdx > totalCount {
			endIdx = totalCount
		}
		items = items[startIdx:endIdx]
	}

	// Convert to DTOs
	itemDTOs := make([]InventoryItemDTO, len(items))
	for i, item := range items {
		itemDTOs[i] = s.convertDomainToDTO(item)
	}

	hasMore := req.Offset+len(items) < totalCount

	return &GetItemsByCategoryResult{
		Items:      itemDTOs,
		TotalCount: totalCount,
		HasMore:    hasMore,
		Message:    fmt.Sprintf("Found %d items in category %s", totalCount, req.Category.String()),
	}, nil
}

// CleanupExpiredReservations removes expired reservations across all items
func (s *inventoryService) CleanupExpiredReservations(ctx context.Context) (*CleanupResult, error) {
	s.logger.Info("Starting cleanup of expired reservations")

	// Get all items that might have reservations
	items, err := s.repository.FindAvailableItems()
	if err != nil {
		s.logger.Error("Failed to find items for cleanup", "error", err)
		return nil, fmt.Errorf("failed to find items: %w", err)
	}

	totalCleaned := 0
	affectedItems := make([]string, 0)

	for _, item := range items {
		expiredOrders := item.CleanupExpiredReservations()
		if len(expiredOrders) > 0 {
			totalCleaned += len(expiredOrders)
			affectedItems = append(affectedItems, item.SKU())

			// Save updated item
			if err := s.repository.Save(item); err != nil {
				s.logger.Error("Failed to save item after cleanup",
					"sku", item.SKU(),
					"error", err)
				continue
			}

			s.logger.Debug("Cleaned expired reservations",
				"sku", item.SKU(),
				"expiredOrders", expiredOrders)
		}
	}

	s.logger.Info("Cleanup completed",
		"totalCleaned", totalCleaned,
		"affectedItems", len(affectedItems))

	return &CleanupResult{
		CleanedReservations: totalCleaned,
		AffectedItems:       affectedItems,
		Message:             fmt.Sprintf("Cleaned %d expired reservations from %d items", totalCleaned, len(affectedItems)),
	}, nil
}

// Helper function for max
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// Helper methods

func (s *inventoryService) validateAvailabilityCheck(item ItemAvailabilityCheck) error {
	if item.SKU == "" {
		return fmt.Errorf("SKU is required")
	}
	if item.Quantity <= 0 {
		return fmt.Errorf("quantity must be positive")
	}
	return nil
}

func (s *inventoryService) validateReserveItemsRequest(req ReserveItemsRequest) error {
	if req.OrderID == "" {
		return fmt.Errorf("order ID is required")
	}
	if len(req.Items) == 0 {
		return fmt.Errorf("at least one item is required")
	}
	if req.ReservationDurationMinutes <= 0 {
		req.ReservationDurationMinutes = s.config.Inventory.MaxReservationTimeMin
	}
	if req.ReservationDurationMinutes > s.config.Inventory.MaxReservationTimeMin {
		return fmt.Errorf("reservation duration exceeds maximum allowed (%d minutes)",
			s.config.Inventory.MaxReservationTimeMin)
	}

	for _, item := range req.Items {
		if item.SKU == "" {
			return fmt.Errorf("SKU is required for all items")
		}
		if item.Quantity <= 0 {
			return fmt.Errorf("quantity must be positive for all items")
		}
	}

	return nil
}

func (s *inventoryService) generateReservationID(orderID string) string {
	timestamp := time.Now().Unix()
	return fmt.Sprintf("res_%s_%d", orderID, timestamp)
}

func (s *inventoryService) releasePartialReservations(orderID string, results []ItemReservationResult) {
	for _, result := range results {
		if result.Reserved {
			item, err := s.repository.FindBySKU(result.SKU)
			if err != nil || item == nil {
				continue
			}

			if err := item.ReleaseReservation(orderID); err != nil {
				s.logger.Error("Failed to release partial reservation",
					"sku", result.SKU,
					"orderID", orderID,
					"error", err)
				continue
			}

			s.repository.Save(item)
		}
	}
}

// convertDomainToDTO converts a domain InventoryItem to DTO
func (s *inventoryService) convertDomainToDTO(item *domain.InventoryItem) InventoryItemDTO {
	return InventoryItemDTO{
		ID:             item.ID(),
		SKU:            item.SKU(),
		Name:           item.Name(),
		Description:    item.Description(),
		Category:       item.Category(),
		StockLevel:     item.StockLevel(),
		ReservedStock:  item.ReservedStock(),
		TotalStock:     item.TotalStock(),
		MinStockLevel:  item.MinStockLevel(),
		MaxStockLevel:  item.MaxStockLevel(),
		UnitPrice:      item.UnitPrice(),
		Weight:         item.Weight(),
		Dimensions:     item.Dimensions(),
		Specifications: item.Specifications(),
		CreatedAt:      item.CreatedAt(),
		UpdatedAt:      item.UpdatedAt(),
		Version:        item.Version(),
		Status:         item.Status(),
	}
}
