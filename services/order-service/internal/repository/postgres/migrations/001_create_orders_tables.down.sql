-- Drop triggers
DROP TRIGGER IF EXISTS update_orders_updated_at ON orders;

-- Drop trigger function
DROP FUNCTION IF EXISTS update_updated_at_column();

-- Drop indexes
DROP INDEX IF EXISTS idx_orders_status_created;
DROP INDEX IF EXISTS idx_orders_created_date;
DROP INDEX IF EXISTS idx_order_items_item_id;
DROP INDEX IF EXISTS idx_order_items_order_id;
DROP INDEX IF EXISTS idx_orders_paid_at;
DROP INDEX IF EXISTS idx_orders_created_at;
DROP INDEX IF EXISTS idx_orders_status;
DROP INDEX IF EXISTS idx_orders_user_id;

-- Drop tables (order_items first due to foreign key constraint)
DROP TABLE IF EXISTS order_items;
DROP TABLE IF EXISTS orders;