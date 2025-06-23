package handlers

import (
	"context"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/amiosamu/rocket-science/services/inventory-service/internal/domain"
	"github.com/amiosamu/rocket-science/services/inventory-service/internal/service"
	pb "github.com/amiosamu/rocket-science/services/inventory-service/proto/inventory"
)

// InventoryHandler implements the InventoryServiceServer interface from protobuf
// It serves as the adapter between gRPC transport and our business service
type InventoryHandler struct {
	pb.UnimplementedInventoryServiceServer // Embedding for forward compatibility
	inventoryService service.InventoryService
	logger           *slog.Logger
}

// NewInventoryHandler creates a new gRPC inventory handler
func NewInventoryHandler(inventoryService service.InventoryService, logger *slog.Logger) *InventoryHandler {
	return &InventoryHandler{
		inventoryService: inventoryService,
		logger:           logger,
	}
}

// CheckAvailability verifies if requested items are available in stock
func (h *InventoryHandler) CheckAvailability(ctx context.Context, req *pb.CheckAvailabilityRequest) (*pb.CheckAvailabilityResponse, error) {
	h.logger.Info("gRPC CheckAvailability called", "itemCount", len(req.Items))

	// Validate request
	if err := h.validateCheckAvailabilityRequest(req); err != nil {
		h.logger.Warn("Invalid CheckAvailability request", "error", err)
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}

	// Convert protobuf to service request
	serviceReq := h.convertToCheckAvailabilityRequest(req)

	// Call business service
	result, err := h.inventoryService.CheckAvailability(ctx, serviceReq)
	if err != nil {
		h.logger.Error("Inventory service error", "error", err)
		return nil, status.Errorf(codes.Internal, "availability check failed: %v", err)
	}

	// Convert service result to protobuf response
	response := h.convertToCheckAvailabilityResponse(result)

	h.logger.Info("CheckAvailability completed", "allAvailable", response.AllAvailable)
	return response, nil
}

// ReserveItems creates reservations for items in an order
func (h *InventoryHandler) ReserveItems(ctx context.Context, req *pb.ReserveItemsRequest) (*pb.ReserveItemsResponse, error) {
	h.logger.Info("gRPC ReserveItems called", 
		"orderID", req.OrderId,
		"itemCount", len(req.Items))

	// Validate request
	if err := h.validateReserveItemsRequest(req); err != nil {
		h.logger.Warn("Invalid ReserveItems request", "error", err)
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}

	// Convert protobuf to service request
	serviceReq := h.convertToReserveItemsRequest(req)

	// Call business service
	result, err := h.inventoryService.ReserveItems(ctx, serviceReq)
	if err != nil {
		h.logger.Error("Reserve items service error", "error", err)
		return nil, status.Errorf(codes.Internal, "reservation failed: %v", err)
	}

	// Convert service result to protobuf response
	response := h.convertToReserveItemsResponse(result)

	h.logger.Info("ReserveItems completed", 
		"success", response.Success,
		"reservationID", response.ReservationId)
	return response, nil
}

// ConfirmReservation confirms reserved items (after payment success)
func (h *InventoryHandler) ConfirmReservation(ctx context.Context, req *pb.ConfirmReservationRequest) (*pb.ConfirmReservationResponse, error) {
	h.logger.Info("gRPC ConfirmReservation called", 
		"orderID", req.OrderId,
		"reservationID", req.ReservationId)

	// Validate request
	if err := h.validateConfirmReservationRequest(req); err != nil {
		h.logger.Warn("Invalid ConfirmReservation request", "error", err)
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}

	// Convert protobuf to service request
	serviceReq := service.ConfirmReservationRequest{
		OrderID:       req.OrderId,
		ReservationID: req.ReservationId,
	}

	// Call business service
	result, err := h.inventoryService.ConfirmReservation(ctx, serviceReq)
	if err != nil {
		h.logger.Error("Confirm reservation service error", "error", err)
		return nil, status.Errorf(codes.Internal, "confirmation failed: %v", err)
	}

	// Convert service result to protobuf response
	response := h.convertToConfirmReservationResponse(result)

	h.logger.Info("ConfirmReservation completed", "success", response.Success)
	return response, nil
}

// ReleaseReservation releases reserved items (if payment fails)
func (h *InventoryHandler) ReleaseReservation(ctx context.Context, req *pb.ReleaseReservationRequest) (*pb.ReleaseReservationResponse, error) {
	h.logger.Info("gRPC ReleaseReservation called", 
		"orderID", req.OrderId,
		"reservationID", req.ReservationId,
		"reason", req.Reason)

	// Validate request
	if err := h.validateReleaseReservationRequest(req); err != nil {
		h.logger.Warn("Invalid ReleaseReservation request", "error", err)
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}

	// Convert protobuf to service request
	serviceReq := service.ReleaseReservationRequest{
		OrderID:       req.OrderId,
		ReservationID: req.ReservationId,
		Reason:        req.Reason,
	}

	// Call business service
	result, err := h.inventoryService.ReleaseReservation(ctx, serviceReq)
	if err != nil {
		h.logger.Error("Release reservation service error", "error", err)
		return nil, status.Errorf(codes.Internal, "release failed: %v", err)
	}

	// Convert service result to protobuf response
	response := h.convertToReleaseReservationResponse(result)

	h.logger.Info("ReleaseReservation completed", "success", response.Success)
	return response, nil
}

// GetItem retrieves details of a specific inventory item
func (h *InventoryHandler) GetItem(ctx context.Context, req *pb.GetItemRequest) (*pb.GetItemResponse, error) {
	h.logger.Debug("gRPC GetItem called")

	// Validate request
	if err := h.validateGetItemRequest(req); err != nil {
		h.logger.Warn("Invalid GetItem request", "error", err)
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}

	// Convert protobuf to service request
	serviceReq := h.convertToGetItemRequest(req)

	// Call business service
	result, err := h.inventoryService.GetItem(ctx, serviceReq)
	if err != nil {
		h.logger.Error("Get item service error", "error", err)
		return nil, status.Errorf(codes.Internal, "get item failed: %v", err)
	}

	// Convert service result to protobuf response
	response := h.convertToGetItemResponse(result)

	h.logger.Debug("GetItem completed", "found", response.Found)
	return response, nil
}

// SearchItems searches for items by name, SKU, or category
func (h *InventoryHandler) SearchItems(ctx context.Context, req *pb.SearchItemsRequest) (*pb.SearchItemsResponse, error) {
	h.logger.Debug("gRPC SearchItems called", 
		"query", req.Query,
		"category", req.Category)

	// Convert protobuf to service request
	serviceReq := h.convertToSearchItemsRequest(req)

	// Call business service
	result, err := h.inventoryService.SearchItems(ctx, serviceReq)
	if err != nil {
		h.logger.Error("Search items service error", "error", err)
		return nil, status.Errorf(codes.Internal, "search failed: %v", err)
	}

	// Convert service result to protobuf response
	response := h.convertToSearchItemsResponse(result)

	h.logger.Debug("SearchItems completed", "itemsFound", len(response.Items))
	return response, nil
}

// GetLowStockItems retrieves items below minimum stock threshold
func (h *InventoryHandler) GetLowStockItems(ctx context.Context, req *pb.GetLowStockItemsRequest) (*pb.GetLowStockItemsResponse, error) {
	h.logger.Debug("gRPC GetLowStockItems called", "category", req.Category)

	// Convert protobuf to service request
	serviceReq := h.convertToGetLowStockItemsRequest(req)

	// Call business service
	result, err := h.inventoryService.GetLowStockItems(ctx, serviceReq)
	if err != nil {
		h.logger.Error("Get low stock items service error", "error", err)
		return nil, status.Errorf(codes.Internal, "get low stock items failed: %v", err)
	}

	// Convert service result to protobuf response
	response := h.convertToGetLowStockItemsResponse(result)

	h.logger.Debug("GetLowStockItems completed", "itemsFound", len(response.Items))
	return response, nil
}

// UpdateStock adds or removes stock (admin operation)
func (h *InventoryHandler) UpdateStock(ctx context.Context, req *pb.UpdateStockRequest) (*pb.UpdateStockResponse, error) {
	h.logger.Info("gRPC UpdateStock called", 
		"sku", req.Sku,
		"quantityChange", req.QuantityChange,
		"updatedBy", req.UpdatedBy)

	// Validate request
	if err := h.validateUpdateStockRequest(req); err != nil {
		h.logger.Warn("Invalid UpdateStock request", "error", err)
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}

	// Convert protobuf to service request
	serviceReq := service.UpdateStockRequest{
		SKU:            req.Sku,
		QuantityChange: int(req.QuantityChange),
		Reason:         req.Reason,
		UpdatedBy:      req.UpdatedBy,
	}

	// Call business service
	result, err := h.inventoryService.UpdateStock(ctx, serviceReq)
	if err != nil {
		h.logger.Error("Update stock service error", "error", err)
		return nil, status.Errorf(codes.Internal, "update stock failed: %v", err)
	}

	// Convert service result to protobuf response
	response := h.convertToUpdateStockResponse(result)

	h.logger.Info("UpdateStock completed", "success", response.Success)
	return response, nil
}

// GetItemsByCategory retrieves items in a specific category
func (h *InventoryHandler) GetItemsByCategory(ctx context.Context, req *pb.GetItemsByCategoryRequest) (*pb.GetItemsByCategoryResponse, error) {
	h.logger.Debug("gRPC GetItemsByCategory called", "category", req.Category)

	// Convert protobuf to service request
	serviceReq := h.convertToGetItemsByCategoryRequest(req)

	// Call business service
	result, err := h.inventoryService.GetItemsByCategory(ctx, serviceReq)
	if err != nil {
		h.logger.Error("Get items by category service error", "error", err)
		return nil, status.Errorf(codes.Internal, "get items by category failed: %v", err)
	}

	// Convert service result to protobuf response
	response := h.convertToGetItemsByCategoryResponse(result)

	h.logger.Debug("GetItemsByCategory completed", "itemsFound", len(response.Items))
	return response, nil
}

// Validation methods

func (h *InventoryHandler) validateCheckAvailabilityRequest(req *pb.CheckAvailabilityRequest) error {
	if len(req.Items) == 0 {
		return status.Error(codes.InvalidArgument, "at least one item is required")
	}

	for i, item := range req.Items {
		if item.Sku == "" {
			return status.Errorf(codes.InvalidArgument, "item %d: SKU is required", i)
		}
		if item.Quantity <= 0 {
			return status.Errorf(codes.InvalidArgument, "item %d: quantity must be positive", i)
		}
	}

	return nil
}

func (h *InventoryHandler) validateReserveItemsRequest(req *pb.ReserveItemsRequest) error {
	if req.OrderId == "" {
		return status.Error(codes.InvalidArgument, "order ID is required")
	}
	if len(req.Items) == 0 {
		return status.Error(codes.InvalidArgument, "at least one item is required")
	}
	if req.ReservationDurationMinutes <= 0 {
		return status.Error(codes.InvalidArgument, "reservation duration must be positive")
	}

	for i, item := range req.Items {
		if item.Sku == "" {
			return status.Errorf(codes.InvalidArgument, "item %d: SKU is required", i)
		}
		if item.Quantity <= 0 {
			return status.Errorf(codes.InvalidArgument, "item %d: quantity must be positive", i)
		}
	}

	return nil
}

func (h *InventoryHandler) validateConfirmReservationRequest(req *pb.ConfirmReservationRequest) error {
	if req.OrderId == "" {
		return status.Error(codes.InvalidArgument, "order ID is required")
	}
	if req.ReservationId == "" {
		return status.Error(codes.InvalidArgument, "reservation ID is required")
	}
	return nil
}

func (h *InventoryHandler) validateReleaseReservationRequest(req *pb.ReleaseReservationRequest) error {
	if req.OrderId == "" {
		return status.Error(codes.InvalidArgument, "order ID is required")
	}
	if req.ReservationId == "" {
		return status.Error(codes.InvalidArgument, "reservation ID is required")
	}
	if req.Reason == "" {
		return status.Error(codes.InvalidArgument, "reason is required")
	}
	return nil
}

func (h *InventoryHandler) validateGetItemRequest(req *pb.GetItemRequest) error {
	switch req.Identifier.(type) {
	case *pb.GetItemRequest_ItemId:
		if req.GetItemId() == "" {
			return status.Error(codes.InvalidArgument, "item ID cannot be empty")
		}
	case *pb.GetItemRequest_Sku:
		if req.GetSku() == "" {
			return status.Error(codes.InvalidArgument, "SKU cannot be empty")
		}
	default:
		return status.Error(codes.InvalidArgument, "either item ID or SKU must be provided")
	}
	return nil
}

func (h *InventoryHandler) validateUpdateStockRequest(req *pb.UpdateStockRequest) error {
	if req.Sku == "" {
		return status.Error(codes.InvalidArgument, "SKU is required")
	}
	if req.QuantityChange == 0 {
		return status.Error(codes.InvalidArgument, "quantity change cannot be zero")
	}
	if req.Reason == "" {
		return status.Error(codes.InvalidArgument, "reason is required")
	}
	if req.UpdatedBy == "" {
		return status.Error(codes.InvalidArgument, "updated by is required")
	}
	return nil
}

// Conversion methods: Protobuf -> Service DTOs

func (h *InventoryHandler) convertToCheckAvailabilityRequest(req *pb.CheckAvailabilityRequest) service.CheckAvailabilityRequest {
	items := make([]service.ItemAvailabilityCheck, len(req.Items))
	for i, item := range req.Items {
		items[i] = service.ItemAvailabilityCheck{
			SKU:      item.Sku,
			Quantity: int(item.Quantity),
		}
	}

	return service.CheckAvailabilityRequest{
		Items: items,
	}
}

func (h *InventoryHandler) convertToReserveItemsRequest(req *pb.ReserveItemsRequest) service.ReserveItemsRequest {
	items := make([]service.ItemReservationRequest, len(req.Items))
	for i, item := range req.Items {
		items[i] = service.ItemReservationRequest{
			SKU:      item.Sku,
			Quantity: int(item.Quantity),
		}
	}

	return service.ReserveItemsRequest{
		OrderID:                   req.OrderId,
		Items:                     items,
		ReservationDurationMinutes: int(req.ReservationDurationMinutes),
	}
}

func (h *InventoryHandler) convertToGetItemRequest(req *pb.GetItemRequest) service.GetItemRequest {
	switch req.Identifier.(type) {
	case *pb.GetItemRequest_ItemId:
		return service.GetItemRequest{
			ItemID: req.GetItemId(),
		}
	case *pb.GetItemRequest_Sku:
		return service.GetItemRequest{
			SKU: req.GetSku(),
		}
	default:
		return service.GetItemRequest{}
	}
}

func (h *InventoryHandler) convertToSearchItemsRequest(req *pb.SearchItemsRequest) service.SearchItemsRequest {
	serviceReq := service.SearchItemsRequest{
		Query:         req.Query,
		AvailableOnly: req.AvailableOnly,
		Limit:         int(req.Limit),
		Offset:        int(req.Offset),
	}

	// Convert category if provided
	if req.Category != pb.ItemCategory_ITEM_CATEGORY_UNSPECIFIED {
		category := h.convertProtoToDomainCategory(req.Category)
		serviceReq.Category = &category
	}

	// Set default limit if not provided
	if serviceReq.Limit <= 0 {
		serviceReq.Limit = 50
	}

	return serviceReq
}

func (h *InventoryHandler) convertToGetLowStockItemsRequest(req *pb.GetLowStockItemsRequest) service.GetLowStockItemsRequest {
	serviceReq := service.GetLowStockItemsRequest{}

	// Convert category if provided
	if req.Category != pb.ItemCategory_ITEM_CATEGORY_UNSPECIFIED {
		category := h.convertProtoToDomainCategory(req.Category)
		serviceReq.Category = &category
	}

	// Convert threshold override if provided
	if req.ThresholdOverride > 0 {
		threshold := int(req.ThresholdOverride)
		serviceReq.ThresholdOverride = &threshold
	}

	return serviceReq
}

func (h *InventoryHandler) convertToGetItemsByCategoryRequest(req *pb.GetItemsByCategoryRequest) service.GetItemsByCategoryRequest {
	serviceReq := service.GetItemsByCategoryRequest{
		Category:      h.convertProtoToDomainCategory(req.Category),
		AvailableOnly: req.AvailableOnly,
		Limit:         int(req.Limit),
		Offset:        int(req.Offset),
	}

	// Set default limit if not provided
	if serviceReq.Limit <= 0 {
		serviceReq.Limit = 50
	}

	return serviceReq
}

// Conversion methods: Service DTOs -> Protobuf responses

func (h *InventoryHandler) convertToCheckAvailabilityResponse(result *service.CheckAvailabilityResult) *pb.CheckAvailabilityResponse {
	results := make([]*pb.ItemAvailabilityResult, len(result.Results))
	for i, item := range result.Results {
		results[i] = &pb.ItemAvailabilityResult{
			Sku:               item.SKU,
			Name:              item.Name,
			Available:         item.Available,
			RequestedQuantity: int32(item.RequestedQuantity),
			AvailableQuantity: int32(item.AvailableQuantity),
			ReservedQuantity:  int32(item.ReservedQuantity),
			Reason:            item.Reason,
		}
	}

	return &pb.CheckAvailabilityResponse{
		AllAvailable: result.AllAvailable,
		Results:      results,
		Message:      result.Message,
	}
}

func (h *InventoryHandler) convertToReserveItemsResponse(result *service.ReserveItemsResult) *pb.ReserveItemsResponse {
	results := make([]*pb.ItemReservationResult, len(result.Results))
	for i, item := range result.Results {
		results[i] = &pb.ItemReservationResult{
			Sku:           item.SKU,
			Name:          item.Name,
			Reserved:      item.Reserved,
			Quantity:      int32(item.Quantity),
			ReservationId: item.ReservationID,
			Reason:        item.Reason,
		}
	}

	return &pb.ReserveItemsResponse{
		Success:       result.Success,
		ReservationId: result.ReservationID,
		Results:       results,
		ExpiresAt:     timestamppb.New(result.ExpiresAt),
		Message:       result.Message,
	}
}

func (h *InventoryHandler) convertToConfirmReservationResponse(result *service.ConfirmReservationResult) *pb.ConfirmReservationResponse {
	results := make([]*pb.ItemConfirmationResult, len(result.Results))
	for i, item := range result.Results {
		results[i] = &pb.ItemConfirmationResult{
			Sku:       item.SKU,
			Name:      item.Name,
			Confirmed: item.Confirmed,
			Quantity:  int32(item.Quantity),
			Reason:    item.Reason,
		}
	}

	return &pb.ConfirmReservationResponse{
		Success:     result.Success,
		Results:     results,
		ConfirmedAt: timestamppb.New(result.ConfirmedAt),
		Message:     result.Message,
	}
}

func (h *InventoryHandler) convertToReleaseReservationResponse(result *service.ReleaseReservationResult) *pb.ReleaseReservationResponse {
	results := make([]*pb.ItemReleaseResult, len(result.Results))
	for i, item := range result.Results {
		results[i] = &pb.ItemReleaseResult{
			Sku:      item.SKU,
			Name:     item.Name,
			Released: item.Released,
			Quantity: int32(item.Quantity),
			Reason:   item.Reason,
		}
	}

	return &pb.ReleaseReservationResponse{
		Success:    result.Success,
		Results:    results,
		ReleasedAt: timestamppb.New(result.ReleasedAt),
		Message:    result.Message,
	}
}

func (h *InventoryHandler) convertToGetItemResponse(result *service.GetItemResult) *pb.GetItemResponse {
	response := &pb.GetItemResponse{
		Found:   result.Found,
		Message: result.Message,
	}

	if result.Item != nil {
		response.Item = h.convertInventoryItemToProto(*result.Item)
	}

	return response
}

func (h *InventoryHandler) convertToSearchItemsResponse(result *service.SearchItemsResult) *pb.SearchItemsResponse {
	items := make([]*pb.InventoryItem, len(result.Items))
	for i, item := range result.Items {
		items[i] = h.convertInventoryItemToProto(item)
	}

	return &pb.SearchItemsResponse{
		Items:      items,
		TotalCount: int32(result.TotalCount),
		HasMore:    result.HasMore,
		Message:    result.Message,
	}
}

func (h *InventoryHandler) convertToGetLowStockItemsResponse(result *service.GetLowStockItemsResult) *pb.GetLowStockItemsResponse {
	items := make([]*pb.LowStockItem, len(result.Items))
	for i, item := range result.Items {
		items[i] = &pb.LowStockItem{
			Item:             h.convertInventoryItemToProto(item.Item),
			ShortageQuantity: int32(item.ShortageQuantity),
			DaysOfStock:      int32(item.DaysOfStock),
		}
	}

	return &pb.GetLowStockItemsResponse{
		Items:      items,
		TotalCount: int32(result.TotalCount),
		Message:    result.Message,
	}
}

func (h *InventoryHandler) convertToUpdateStockResponse(result *service.UpdateStockResult) *pb.UpdateStockResponse {
	return &pb.UpdateStockResponse{
		Success:       result.Success,
		OldStockLevel: int32(result.OldStockLevel),
		NewStockLevel: int32(result.NewStockLevel),
		UpdatedAt:     timestamppb.New(result.UpdatedAt),
		Message:       result.Message,
	}
}

func (h *InventoryHandler) convertToGetItemsByCategoryResponse(result *service.GetItemsByCategoryResult) *pb.GetItemsByCategoryResponse {
	items := make([]*pb.InventoryItem, len(result.Items))
	for i, item := range result.Items {
		items[i] = h.convertInventoryItemToProto(item)
	}

	return &pb.GetItemsByCategoryResponse{
		Items:      items,
		TotalCount: int32(result.TotalCount),
		HasMore:    result.HasMore,
		Message:    result.Message,
	}
}

// Helper conversion methods

func (h *InventoryHandler) convertInventoryItemToProto(item service.InventoryItemDTO) *pb.InventoryItem {
	return &pb.InventoryItem{
		Id:          item.ID,
		Sku:         item.SKU,
		Name:        item.Name,
		Description: item.Description,
		Category:    h.convertDomainToProtoCategory(item.Category),
		StockLevel:  int32(item.StockLevel),
		ReservedStock: int32(item.ReservedStock),
		TotalStock:  int32(item.TotalStock),
		MinStockLevel: int32(item.MinStockLevel),
		MaxStockLevel: int32(item.MaxStockLevel),
		UnitPrice: &pb.Money{
			Amount:   item.UnitPrice.Amount,
			Currency: item.UnitPrice.Currency,
		},
		Weight: item.Weight,
		Dimensions: &pb.Dimensions{
			Length: item.Dimensions.Length,
			Width:  item.Dimensions.Width,
			Height: item.Dimensions.Height,
		},
		Specifications: item.Specifications,
		CreatedAt:     timestamppb.New(item.CreatedAt),
		UpdatedAt:     timestamppb.New(item.UpdatedAt),
		Version:       int32(item.Version),
		Status:        h.convertDomainToProtoStatus(item.Status),
	}
}

func (h *InventoryHandler) convertDomainToProtoCategory(category domain.ItemCategory) pb.ItemCategory {
	switch category {
	case domain.CategoryEngines:
		return pb.ItemCategory_ITEM_CATEGORY_ENGINES
	case domain.CategoryFuelTanks:
		return pb.ItemCategory_ITEM_CATEGORY_FUEL_TANKS
	case domain.CategoryNavigation:
		return pb.ItemCategory_ITEM_CATEGORY_NAVIGATION
	case domain.CategoryStructural:
		return pb.ItemCategory_ITEM_CATEGORY_STRUCTURAL
	case domain.CategoryElectronics:
		return pb.ItemCategory_ITEM_CATEGORY_ELECTRONICS
	case domain.CategoryLifeSupport:
		return pb.ItemCategory_ITEM_CATEGORY_LIFE_SUPPORT
	case domain.CategoryPayload:
		return pb.ItemCategory_ITEM_CATEGORY_PAYLOAD
	case domain.CategoryLandingGear:
		return pb.ItemCategory_ITEM_CATEGORY_LANDING_GEAR
	default:
		return pb.ItemCategory_ITEM_CATEGORY_UNSPECIFIED
	}
}

func (h *InventoryHandler) convertProtoToDomainCategory(category pb.ItemCategory) domain.ItemCategory {
	switch category {
	case pb.ItemCategory_ITEM_CATEGORY_ENGINES:
		return domain.CategoryEngines
	case pb.ItemCategory_ITEM_CATEGORY_FUEL_TANKS:
		return domain.CategoryFuelTanks
	case pb.ItemCategory_ITEM_CATEGORY_NAVIGATION:
		return domain.CategoryNavigation
	case pb.ItemCategory_ITEM_CATEGORY_STRUCTURAL:
		return domain.CategoryStructural
	case pb.ItemCategory_ITEM_CATEGORY_ELECTRONICS:
		return domain.CategoryElectronics
	case pb.ItemCategory_ITEM_CATEGORY_LIFE_SUPPORT:
		return domain.CategoryLifeSupport
	case pb.ItemCategory_ITEM_CATEGORY_PAYLOAD:
		return domain.CategoryPayload
	case pb.ItemCategory_ITEM_CATEGORY_LANDING_GEAR:
		return domain.CategoryLandingGear
	default:
		return domain.CategoryEngines // Default fallback
	}
}

func (h *InventoryHandler) convertDomainToProtoStatus(status domain.ItemStatus) pb.ItemStatus {
	switch status {
	case domain.ItemStatusActive:
		return pb.ItemStatus_ITEM_STATUS_ACTIVE
	case domain.ItemStatusDiscontinued:
		return pb.ItemStatus_ITEM_STATUS_DISCONTINUED
	case domain.ItemStatusOutOfStock:
		return pb.ItemStatus_ITEM_STATUS_OUT_OF_STOCK
	case domain.ItemStatusBackordered:
		return pb.ItemStatus_ITEM_STATUS_BACKORDERED
	case domain.ItemStatusIncoming:
		return pb.ItemStatus_ITEM_STATUS_INCOMING
	default:
		return pb.ItemStatus_ITEM_STATUS_UNSPECIFIED
	}
}