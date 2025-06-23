package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	platformError "github.com/amiosamu/rocket-science/shared/platform/errors"
	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	"github.com/amiosamu/rocket-science/services/order-service/internal/domain"
	"github.com/amiosamu/rocket-science/services/order-service/internal/repository/interfaces"
)

// OrderRepository implements the OrderRepository interface using PostgreSQL
type OrderRepository struct {
	db *sqlx.DB
}

// NewOrderRepository creates a new PostgreSQL order repository
func NewOrderRepository(db *sqlx.DB) interfaces.OrderRepository {
	return &OrderRepository{
		db: db,
	}
}

// Create creates a new order with its items in a transaction
func (r *OrderRepository) Create(ctx context.Context, order *domain.Order) error {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return platformError.Wrap(err, "failed to begin transaction")
	}
	defer tx.Rollback()

	// Insert order
	orderQuery := `
		INSERT INTO orders (id, user_id, status, total_amount, currency, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`

	_, err = tx.ExecContext(ctx, orderQuery,
		order.ID, order.UserID, order.Status, order.TotalAmount,
		order.Currency, order.CreatedAt, order.UpdatedAt)
	if err != nil {
		return platformError.Wrap(err, "failed to insert order")
	}

	// Insert order items
	if len(order.Items) > 0 {
		itemQuery := `
			INSERT INTO order_items (id, order_id, item_id, item_name, quantity, unit_price, total, created_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`

		for _, item := range order.Items {
			_, err = tx.ExecContext(ctx, itemQuery,
				item.ID, item.OrderID, item.ItemID, item.ItemName,
				item.Quantity, item.UnitPrice, item.Total, item.CreatedAt)
			if err != nil {
				return platformError.Wrap(err, "failed to insert order item")
			}
		}
	}

	return tx.Commit()
}

// GetByID retrieves an order by its ID, including items
func (r *OrderRepository) GetByID(ctx context.Context, id uuid.UUID) (*domain.Order, error) {
	// Get order
	orderQuery := `
		SELECT id, user_id, status, total_amount, currency, created_at, updated_at,
			   paid_at, assembled_at, completed_at
		FROM orders 
		WHERE id = $1 AND deleted_at IS NULL`

	order := &domain.Order{}
	err := r.db.GetContext(ctx, order, orderQuery, id)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, platformError.NewNotFound("order not found")
		}
		return nil, platformError.Wrap(err, "failed to get order")
	}

	// Get order items
	itemsQuery := `
		SELECT id, order_id, item_id, item_name, quantity, unit_price, total, created_at
		FROM order_items
		WHERE order_id = $1
		ORDER BY created_at`

	items := []domain.OrderItem{}
	err = r.db.SelectContext(ctx, &items, itemsQuery, id)
	if err != nil {
		return nil, platformError.Wrap(err, "failed to get order items")
	}

	order.Items = items
	return order, nil
}

// GetByUserID retrieves orders for a specific user with pagination
func (r *OrderRepository) GetByUserID(ctx context.Context, userID uuid.UUID, limit, offset int) ([]*domain.Order, error) {
	query := `
		SELECT id, user_id, status, total_amount, currency, created_at, updated_at,
			   paid_at, assembled_at, completed_at
		FROM orders 
		WHERE user_id = $1 AND deleted_at IS NULL
		ORDER BY created_at DESC
		LIMIT $2 OFFSET $3`

	orders := []*domain.Order{}
	err := r.db.SelectContext(ctx, &orders, query, userID, limit, offset)
	if err != nil {
		return nil, platformError.Wrap(err, "failed to get orders by user ID")
	}

	// Load items for each order
	for _, order := range orders {
		items := []domain.OrderItem{}
		itemsQuery := `
			SELECT id, order_id, item_id, item_name, quantity, unit_price, total, created_at
			FROM order_items
			WHERE order_id = $1
			ORDER BY created_at`

		err = r.db.SelectContext(ctx, &items, itemsQuery, order.ID)
		if err != nil {
			return nil, platformError.Wrap(err, "failed to get order items")
		}
		order.Items = items
	}

	return orders, nil
}

// Update updates an existing order
func (r *OrderRepository) Update(ctx context.Context, order *domain.Order) error {
	query := `
		UPDATE orders 
		SET status = $2, total_amount = $3, updated_at = $4,
			paid_at = $5, assembled_at = $6, completed_at = $7
		WHERE id = $1 AND deleted_at IS NULL`

	result, err := r.db.ExecContext(ctx, query,
		order.ID, order.Status, order.TotalAmount, order.UpdatedAt,
		order.PaidAt, order.AssembledAt, order.CompletedAt)
	if err != nil {
		return platformError.Wrap(err, "failed to update order")
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return platformError.Wrap(err, "failed to get rows affected")
	}

	if rowsAffected == 0 {
		return platformError.NewNotFound("order not found")
	}

	return nil
}

// UpdateStatus updates only the status and related timestamps of an order
func (r *OrderRepository) UpdateStatus(ctx context.Context, id uuid.UUID, status domain.OrderStatus) error {
	now := time.Now()

	query := `
		UPDATE orders 
		SET status = $2, updated_at = $3,
			paid_at = CASE WHEN $2 = 'paid' THEN $4 ELSE paid_at END,
			assembled_at = CASE WHEN $2 = 'assembled' THEN $4 ELSE assembled_at END,
			completed_at = CASE WHEN $2 = 'completed' THEN $4 ELSE completed_at END
		WHERE id = $1 AND deleted_at IS NULL`

	result, err := r.db.ExecContext(ctx, query, id, status, now, now)
	if err != nil {
		return platformError.Wrap(err, "failed to update order status")
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return platformError.Wrap(err, "failed to get rows affected")
	}

	if rowsAffected == 0 {
		return platformError.NewNotFound("order not found")
	}

	return nil
}

// List retrieves orders based on filter criteria with pagination
func (r *OrderRepository) List(ctx context.Context, filter domain.OrderFilter) ([]*domain.Order, error) {
	whereClause := []string{"deleted_at IS NULL"}
	args := []interface{}{}
	argIndex := 1

	if filter.UserID != nil {
		whereClause = append(whereClause, fmt.Sprintf("user_id = $%d", argIndex))
		args = append(args, *filter.UserID)
		argIndex++
	}

	if filter.Status != nil {
		whereClause = append(whereClause, fmt.Sprintf("status = $%d", argIndex))
		args = append(args, *filter.Status)
		argIndex++
	}

	limit := 50 // default limit
	if filter.Limit > 0 {
		limit = filter.Limit
	}

	offset := 0
	if filter.Offset > 0 {
		offset = filter.Offset
	}

	query := fmt.Sprintf(`
		SELECT id, user_id, status, total_amount, currency, created_at, updated_at,
			   paid_at, assembled_at, completed_at
		FROM orders 
		WHERE %s
		ORDER BY created_at DESC
		LIMIT $%d OFFSET $%d`,
		strings.Join(whereClause, " AND "), argIndex, argIndex+1)

	args = append(args, limit, offset)

	orders := []*domain.Order{}
	err := r.db.SelectContext(ctx, &orders, query, args...)
	if err != nil {
		return nil, platformError.Wrap(err, "failed to list orders")
	}

	// Load items for each order
	for _, order := range orders {
		items := []domain.OrderItem{}
		itemsQuery := `
			SELECT id, order_id, item_id, item_name, quantity, unit_price, total, created_at
			FROM order_items
			WHERE order_id = $1
			ORDER BY created_at`

		err = r.db.SelectContext(ctx, &items, itemsQuery, order.ID)
		if err != nil {
			return nil, platformError.Wrap(err, "failed to get order items")
		}
		order.Items = items
	}

	return orders, nil
}

// Count returns the total number of orders matching the filter criteria
func (r *OrderRepository) Count(ctx context.Context, filter domain.OrderFilter) (int, error) {
	whereClause := []string{"deleted_at IS NULL"}
	args := []interface{}{}
	argIndex := 1

	if filter.UserID != nil {
		whereClause = append(whereClause, fmt.Sprintf("user_id = $%d", argIndex))
		args = append(args, *filter.UserID)
		argIndex++
	}

	if filter.Status != nil {
		whereClause = append(whereClause, fmt.Sprintf("status = $%d", argIndex))
		args = append(args, *filter.Status)
		argIndex++
	}

	query := fmt.Sprintf(`
		SELECT COUNT(*) 
		FROM orders 
		WHERE %s`,
		strings.Join(whereClause, " AND "))

	var count int
	err := r.db.GetContext(ctx, &count, query, args...)
	if err != nil {
		return 0, platformError.Wrap(err, "failed to count orders")
	}

	return count, nil
}

// Delete soft deletes an order
func (r *OrderRepository) Delete(ctx context.Context, id uuid.UUID) error {
	query := `
		UPDATE orders 
		SET deleted_at = $2, updated_at = $2
		WHERE id = $1 AND deleted_at IS NULL`

	result, err := r.db.ExecContext(ctx, query, id, time.Now())
	if err != nil {
		return platformError.Wrap(err, "failed to delete order")
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return platformError.Wrap(err, "failed to get rows affected")
	}

	if rowsAffected == 0 {
		return platformError.NewNotFound("order not found")
	}

	return nil
}

// GetOrderMetrics returns aggregated metrics for monitoring and analytics
func (r *OrderRepository) GetOrderMetrics(ctx context.Context) (*interfaces.OrderMetrics, error) {
	metrics := &interfaces.OrderMetrics{
		OrdersByStatus: make(map[string]int),
	}

	// Get total orders and revenue
	totalQuery := `
		SELECT COUNT(*) as total_orders, 
			   COALESCE(SUM(total_amount), 0) as total_revenue,
			   COALESCE(AVG(total_amount), 0) as average_order_value
		FROM orders 
		WHERE deleted_at IS NULL`

	var totalData struct {
		TotalOrders       int     `db:"total_orders"`
		TotalRevenue      float64 `db:"total_revenue"`
		AverageOrderValue float64 `db:"average_order_value"`
	}

	err := r.db.GetContext(ctx, &totalData, totalQuery)
	if err != nil {
		return nil, platformError.Wrap(err, "failed to get total order metrics")
	}

	metrics.TotalOrders = totalData.TotalOrders
	metrics.TotalRevenue = totalData.TotalRevenue
	metrics.AverageOrderValue = totalData.AverageOrderValue

	// Get orders by status
	statusQuery := `
		SELECT status, COUNT(*) as count
		FROM orders 
		WHERE deleted_at IS NULL
		GROUP BY status`

	statusRows, err := r.db.QueryContext(ctx, statusQuery)
	if err != nil {
		return nil, platformError.Wrap(err, "failed to get orders by status")
	}
	defer statusRows.Close()

	for statusRows.Next() {
		var status string
		var count int
		if err := statusRows.Scan(&status, &count); err != nil {
			return nil, platformError.Wrap(err, "failed to scan status row")
		}
		metrics.OrdersByStatus[status] = count
	}

	// Get today's metrics
	todayQuery := `
		SELECT COUNT(*) as orders_today, 
			   COALESCE(SUM(total_amount), 0) as revenue_today
		FROM orders 
		WHERE deleted_at IS NULL 
		AND DATE(created_at) = CURRENT_DATE`

	var todayData struct {
		OrdersToday  int     `db:"orders_today"`
		RevenueToday float64 `db:"revenue_today"`
	}

	err = r.db.GetContext(ctx, &todayData, todayQuery)
	if err != nil {
		return nil, platformError.Wrap(err, "failed to get today's metrics")
	}

	metrics.OrdersToday = todayData.OrdersToday
	metrics.RevenueToday = todayData.RevenueToday

	return metrics, nil
}
