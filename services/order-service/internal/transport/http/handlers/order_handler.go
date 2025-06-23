package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/amiosamu/rocket-science/services/order-service/internal/domain"
	"github.com/amiosamu/rocket-science/services/order-service/internal/service"
	"github.com/amiosamu/rocket-science/shared/platform/errors"
	"github.com/amiosamu/rocket-science/shared/platform/observability/logging"
	"github.com/amiosamu/rocket-science/shared/platform/observability/tracing"
)

// OrderHandler handles HTTP requests for orders
type OrderHandler struct {
	orderService *service.OrderService
	logger       logging.Logger
}

// NewOrderHandler creates a new order handler
func NewOrderHandler(orderService *service.OrderService, logger logging.Logger) *OrderHandler {
	return &OrderHandler{
		orderService: orderService,
		logger:       logger,
	}
}

// CreateOrder handles POST /orders
func (h *OrderHandler) CreateOrder(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	tracing.AddSpanAttributes(ctx,
		tracing.HTTPMethodKey.String(r.Method),
		tracing.HTTPURLKey.String(r.URL.String()),
	)

	var req CreateOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.logger.Error(ctx, "Failed to decode create order request", err)
		h.respondWithError(w, http.StatusBadRequest, "Invalid JSON payload", err)
		return
	}

	// Validate request
	if err := h.validateCreateOrderRequest(req); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "Invalid request", err)
		return
	}

	// Convert to domain request
	domainReq := domain.CreateOrderRequest{
		UserID: req.UserID,
		Items:  make([]domain.CreateOrderItemRequest, len(req.Items)),
	}

	for i, item := range req.Items {
		domainReq.Items[i] = domain.CreateOrderItemRequest{
			ItemID:   item.ItemID,
			Quantity: item.Quantity,
		}
	}

	// Create order
	order, err := h.orderService.CreateOrder(ctx, domainReq)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	// Convert to response
	response := h.convertOrderToResponse(order)

	h.logger.Info(ctx, "Order created successfully", map[string]interface{}{
		"order_id": order.ID,
		"user_id":  order.UserID,
		"total":    order.TotalAmount,
	})

	h.respondWithJSON(w, http.StatusCreated, response)
}

// GetOrder handles GET /orders/{id}
func (h *OrderHandler) GetOrder(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	orderID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.respondWithError(w, http.StatusBadRequest, "Invalid order ID", err)
		return
	}

	tracing.AddSpanAttributes(ctx, tracing.OrderIDKey.String(orderID.String()))

	order, err := h.orderService.GetOrder(ctx, orderID)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	response := h.convertOrderToResponse(order)
	h.respondWithJSON(w, http.StatusOK, response)
}

// GetUserOrders handles GET /users/{userID}/orders
func (h *OrderHandler) GetUserOrders(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	userID, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil {
		h.respondWithError(w, http.StatusBadRequest, "Invalid user ID", err)
		return
	}

	tracing.AddSpanAttributes(ctx, tracing.UserIDKey.String(userID.String()))

	// Parse query parameters
	limit, offset := h.parsePaginationParams(r)

	orders, err := h.orderService.GetUserOrders(ctx, userID, limit, offset)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	// Convert to response
	response := UserOrdersResponse{
		Orders: make([]OrderResponse, len(orders)),
		Pagination: PaginationResponse{
			Limit:  limit,
			Offset: offset,
			Total:  len(orders), // This could be improved with actual count
		},
	}

	for i, order := range orders {
		response.Orders[i] = h.convertOrderToResponse(order)
	}

	h.respondWithJSON(w, http.StatusOK, response)
}

// ListOrders handles GET /orders
func (h *OrderHandler) ListOrders(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Parse query parameters
	filter := h.parseOrderFilter(r)

	orders, err := h.orderService.ListOrders(ctx, filter)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	// Convert to response
	response := OrderListResponse{
		Orders: make([]OrderResponse, len(orders)),
		Filter: FilterResponse{
			UserID: filter.UserID,
			Status: filter.Status,
			Limit:  filter.Limit,
			Offset: filter.Offset,
		},
	}

	for i, order := range orders {
		response.Orders[i] = h.convertOrderToResponse(order)
	}

	h.respondWithJSON(w, http.StatusOK, response)
}

// UpdateOrderStatus handles PATCH /orders/{id}/status
func (h *OrderHandler) UpdateOrderStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	orderID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		h.respondWithError(w, http.StatusBadRequest, "Invalid order ID", err)
		return
	}

	var req UpdateOrderStatusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.respondWithError(w, http.StatusBadRequest, "Invalid JSON payload", err)
		return
	}

	// Validate status
	if !h.isValidOrderStatus(string(req.Status)) {
		h.respondWithError(w, http.StatusBadRequest, "Invalid order status", nil)
		return
	}

	err = h.orderService.UpdateOrderStatus(ctx, orderID, domain.OrderStatus(req.Status))
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	response := map[string]interface{}{
		"success":    true,
		"message":    "Order status updated successfully",
		"order_id":   orderID,
		"new_status": req.Status,
	}

	h.logger.Info(ctx, "Order status updated", map[string]interface{}{
		"order_id": orderID,
		"status":   req.Status,
	})

	h.respondWithJSON(w, http.StatusOK, response)
}

// GetOrderMetrics handles GET /orders/metrics
func (h *OrderHandler) GetOrderMetrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	metrics, err := h.orderService.GetOrderMetrics(ctx)
	if err != nil {
		h.handleServiceError(w, err)
		return
	}

	response := MetricsResponse{
		TotalOrders:       metrics.TotalOrders,
		TotalRevenue:      metrics.TotalRevenue,
		AverageOrderValue: metrics.AverageOrderValue,
		OrdersByStatus:    metrics.OrdersByStatus,
		OrdersToday:       metrics.OrdersToday,
		RevenueToday:      metrics.RevenueToday,
	}

	h.respondWithJSON(w, http.StatusOK, response)
}

// HealthCheck handles GET /health
func (h *OrderHandler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	response := HealthResponse{
		Status:    "healthy",
		Service:   "order-service",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Version:   "1.0.0",
	}
	h.respondWithJSON(w, http.StatusOK, response)
}

// Private helper methods

func (h *OrderHandler) validateCreateOrderRequest(req CreateOrderRequest) error {
	if req.UserID == uuid.Nil {
		return errors.NewValidation("user_id is required")
	}

	if len(req.Items) == 0 {
		return errors.NewValidation("at least one item is required")
	}

	for i, item := range req.Items {
		if strings.TrimSpace(item.ItemID) == "" {
			return errors.NewValidation("item_id is required for item " + strconv.Itoa(i))
		}
		if item.Quantity <= 0 {
			return errors.NewValidation("quantity must be positive for item " + strconv.Itoa(i))
		}
	}

	return nil
}

func (h *OrderHandler) parsePaginationParams(r *http.Request) (limit, offset int) {
	limit = 50 // default
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsedLimit, err := strconv.Atoi(l); err == nil && parsedLimit > 0 && parsedLimit <= 100 {
			limit = parsedLimit
		}
	}

	offset = 0 // default
	if o := r.URL.Query().Get("offset"); o != "" {
		if parsedOffset, err := strconv.Atoi(o); err == nil && parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	return limit, offset
}

func (h *OrderHandler) parseOrderFilter(r *http.Request) domain.OrderFilter {
	filter := domain.OrderFilter{}

	// Parse user_id filter
	if userIDStr := r.URL.Query().Get("user_id"); userIDStr != "" {
		if userID, err := uuid.Parse(userIDStr); err == nil {
			filter.UserID = &userID
		}
	}

	// Parse status filter
	if statusStr := r.URL.Query().Get("status"); statusStr != "" && h.isValidOrderStatus(statusStr) {
		status := domain.OrderStatus(statusStr)
		filter.Status = &status
	}

	// Parse pagination
	filter.Limit, filter.Offset = h.parsePaginationParams(r)

	return filter
}

func (h *OrderHandler) isValidOrderStatus(status string) bool {
	validStatuses := []string{
		string(domain.StatusPending),
		string(domain.StatusPaid),
		string(domain.StatusAssembled),
		string(domain.StatusCompleted),
		string(domain.StatusCancelled),
		string(domain.StatusFailed),
	}

	for _, validStatus := range validStatuses {
		if status == validStatus {
			return true
		}
	}
	return false
}

func (h *OrderHandler) convertOrderToResponse(order *domain.Order) OrderResponse {
	response := OrderResponse{
		ID:          order.ID,
		UserID:      order.UserID,
		Status:      string(order.Status),
		TotalAmount: order.TotalAmount,
		Currency:    order.Currency,
		CreatedAt:   order.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   order.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		Items:       make([]OrderItemResponse, len(order.Items)),
	}

	// Add optional timestamps
	if order.PaidAt != nil {
		paidAt := order.PaidAt.Format("2006-01-02T15:04:05Z07:00")
		response.PaidAt = &paidAt
	}
	if order.AssembledAt != nil {
		assembledAt := order.AssembledAt.Format("2006-01-02T15:04:05Z07:00")
		response.AssembledAt = &assembledAt
	}
	if order.CompletedAt != nil {
		completedAt := order.CompletedAt.Format("2006-01-02T15:04:05Z07:00")
		response.CompletedAt = &completedAt
	}

	// Convert items
	for i, item := range order.Items {
		response.Items[i] = OrderItemResponse{
			ID:        item.ID,
			ItemID:    item.ItemID,
			ItemName:  item.ItemName,
			Quantity:  item.Quantity,
			UnitPrice: item.UnitPrice,
			Total:     item.Total,
		}
	}

	return response
}

func (h *OrderHandler) respondWithJSON(w http.ResponseWriter, statusCode int, payload interface{}) {
	response, err := json.Marshal(payload)
	if err != nil {
		h.logger.Error(nil, "Failed to marshal JSON response", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	w.Write(response)
}

func (h *OrderHandler) respondWithError(w http.ResponseWriter, statusCode int, message string, err error) {
	errorResponse := ErrorResponse{
		Error:   message,
		Code:    statusCode,
		Details: "",
	}

	if err != nil {
		errorResponse.Details = err.Error()
		h.logger.Error(nil, message, err)
	}

	h.respondWithJSON(w, statusCode, errorResponse)
}

func (h *OrderHandler) handleServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.IsNotFound(err):
		h.respondWithError(w, http.StatusNotFound, "Resource not found", err)
	case errors.IsValidation(err):
		h.respondWithError(w, http.StatusBadRequest, "Validation error", err)
	case errors.IsConflict(err):
		h.respondWithError(w, http.StatusConflict, "Conflict error", err)
	case errors.IsExternal(err):
		h.respondWithError(w, http.StatusBadGateway, "External service error", err)
	default:
		h.logger.Error(nil, "Internal server error", err)
		h.respondWithError(w, http.StatusInternalServerError, "Internal server error", nil)
	}
}
