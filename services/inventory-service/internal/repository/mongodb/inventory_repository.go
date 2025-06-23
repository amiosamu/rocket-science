package mongodb

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/amiosamu/rocket-science/services/inventory-service/internal/config"
	"github.com/amiosamu/rocket-science/services/inventory-service/internal/domain"
)

const (
	// Collection names
	inventoryCollection = "inventory_items"
	
	// Index names
	skuIndex      = "sku_index"
	categoryIndex = "category_index"
	stockIndex    = "stock_index"
	statusIndex   = "status_index"
	textIndex     = "text_index"
)

// MongoInventoryRepository implements the domain.InventoryRepository interface using MongoDB
type MongoInventoryRepository struct {
	client     *mongo.Client
	database   *mongo.Database
	collection *mongo.Collection
	config     *config.Config
	logger     *slog.Logger
	timeout    time.Duration
}

// MongoDB document models - these represent how data is stored in MongoDB
// They're separate from domain models to maintain clean separation

// inventoryItemDoc represents an inventory item document in MongoDB
type inventoryItemDoc struct {
	ID             primitive.ObjectID `bson:"_id,omitempty"`
	ItemID         string             `bson:"item_id"`
	SKU            string             `bson:"sku"`
	Name           string             `bson:"name"`
	Description    string             `bson:"description"`
	Category       int                `bson:"category"`
	StockLevel     int                `bson:"stock_level"`
	ReservedStock  int                `bson:"reserved_stock"`
	TotalStock     int                `bson:"total_stock"`
	MinStockLevel  int                `bson:"min_stock_level"`
	MaxStockLevel  int                `bson:"max_stock_level"`
	Reservations   []reservationDoc   `bson:"reservations"`
	UnitPrice      moneyDoc           `bson:"unit_price"`
	Weight         float64            `bson:"weight"`
	Dimensions     dimensionsDoc      `bson:"dimensions"`
	Specifications map[string]string  `bson:"specifications"`
	CreatedAt      time.Time          `bson:"created_at"`
	UpdatedAt      time.Time          `bson:"updated_at"`
	Version        int                `bson:"version"`
	Status         int                `bson:"status"`
}

// reservationDoc represents a stock reservation in MongoDB
type reservationDoc struct {
	ID         string    `bson:"id"`
	OrderID    string    `bson:"order_id"`
	ItemID     string    `bson:"item_id"`
	Quantity   int       `bson:"quantity"`
	ReservedAt time.Time `bson:"reserved_at"`
	ExpiresAt  time.Time `bson:"expires_at"`
	Status     int       `bson:"status"`
}

// moneyDoc represents currency amounts in MongoDB
type moneyDoc struct {
	Amount   float64 `bson:"amount"`
	Currency string  `bson:"currency"`
}

// dimensionsDoc represents physical dimensions in MongoDB
type dimensionsDoc struct {
	Length float64 `bson:"length"`
	Width  float64 `bson:"width"`
	Height float64 `bson:"height"`
}

// NewMongoInventoryRepository creates a new MongoDB inventory repository
func NewMongoInventoryRepository(cfg *config.Config, logger *slog.Logger) (*MongoInventoryRepository, error) {
	ctx, cancel := context.WithTimeout(context.Background(), cfg.Database.ConnectTimeout)
	defer cancel()

	// Create MongoDB client
	clientOpts := options.Client().
		ApplyURI(cfg.Database.ConnectionURL).
		SetMaxPoolSize(uint64(cfg.Database.MaxPoolSize)).
		SetMinPoolSize(uint64(cfg.Database.MinPoolSize)).
		SetMaxConnIdleTime(cfg.Database.MaxConnIdleTime).
		SetConnectTimeout(cfg.Database.ConnectTimeout)

	client, err := mongo.Connect(ctx, clientOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %w", err)
	}

	// Test the connection
	if err := client.Ping(ctx, nil); err != nil {
		return nil, fmt.Errorf("failed to ping MongoDB: %w", err)
	}

	database := client.Database(cfg.Database.DatabaseName)
	collection := database.Collection(inventoryCollection)

	repo := &MongoInventoryRepository{
		client:     client,
		database:   database,
		collection: collection,
		config:     cfg,
		logger:     logger,
		timeout:    cfg.Database.QueryTimeout,
	}

	// Create indexes
	if err := repo.createIndexes(ctx); err != nil {
		logger.Warn("Failed to create indexes", "error", err)
		// Don't fail - indexes can be created later
	}

	logger.Info("MongoDB inventory repository initialized",
		"database", cfg.Database.DatabaseName,
		"collection", inventoryCollection)

	return repo, nil
}

// Save persists an inventory item to MongoDB
func (r *MongoInventoryRepository) Save(item *domain.InventoryItem) error {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	// Convert domain model to MongoDB document
	doc := r.domainToDocument(item)

	// Use upsert based on item_id
	filter := bson.M{"item_id": item.ID()}
	update := bson.M{"$set": doc}
	opts := options.Update().SetUpsert(true)

	result, err := r.collection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		r.logger.Error("Failed to save inventory item", "error", err, "itemID", item.ID())
		return fmt.Errorf("failed to save inventory item: %w", err)
	}

	r.logger.Debug("Inventory item saved",
		"itemID", item.ID(),
		"sku", item.SKU(),
		"matched", result.MatchedCount,
		"modified", result.ModifiedCount,
		"upserted", result.UpsertedCount)

	return nil
}

// FindByID retrieves an inventory item by its unique identifier
func (r *MongoInventoryRepository) FindByID(id string) (*domain.InventoryItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	filter := bson.M{"item_id": id}
	var doc inventoryItemDoc

	err := r.collection.FindOne(ctx, filter).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil // Item not found
		}
		r.logger.Error("Failed to find inventory item by ID", "error", err, "itemID", id)
		return nil, fmt.Errorf("failed to find inventory item: %w", err)
	}

	// Convert MongoDB document to domain model
	return r.documentToDomain(&doc)
}

// FindBySKU retrieves an inventory item by its SKU
func (r *MongoInventoryRepository) FindBySKU(sku string) (*domain.InventoryItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	filter := bson.M{"sku": sku}
	var doc inventoryItemDoc

	err := r.collection.FindOne(ctx, filter).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil // Item not found
		}
		r.logger.Error("Failed to find inventory item by SKU", "error", err, "sku", sku)
		return nil, fmt.Errorf("failed to find inventory item: %w", err)
	}

	return r.documentToDomain(&doc)
}

// FindByCategory retrieves inventory items by category
func (r *MongoInventoryRepository) FindByCategory(category domain.ItemCategory) ([]*domain.InventoryItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	filter := bson.M{"category": int(category)}
	
	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		r.logger.Error("Failed to find inventory items by category", "error", err, "category", category)
		return nil, fmt.Errorf("failed to find inventory items: %w", err)
	}
	defer cursor.Close(ctx)

	var items []*domain.InventoryItem
	for cursor.Next(ctx) {
		var doc inventoryItemDoc
		if err := cursor.Decode(&doc); err != nil {
			r.logger.Warn("Failed to decode inventory item", "error", err)
			continue
		}

		item, err := r.documentToDomain(&doc)
		if err != nil {
			r.logger.Warn("Failed to convert document to domain", "error", err)
			continue
		}

		items = append(items, item)
	}

	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("cursor error: %w", err)
	}

	return items, nil
}

// FindLowStockItems retrieves items below minimum stock threshold
func (r *MongoInventoryRepository) FindLowStockItems() ([]*domain.InventoryItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	// Find items where stock_level <= min_stock_level
	filter := bson.M{
		"$expr": bson.M{
			"$lte": []interface{}{"$stock_level", "$min_stock_level"},
		},
		"status": int(domain.ItemStatusActive), // Only active items
	}

	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		r.logger.Error("Failed to find low stock items", "error", err)
		return nil, fmt.Errorf("failed to find low stock items: %w", err)
	}
	defer cursor.Close(ctx)

	var items []*domain.InventoryItem
	for cursor.Next(ctx) {
		var doc inventoryItemDoc
		if err := cursor.Decode(&doc); err != nil {
			r.logger.Warn("Failed to decode inventory item", "error", err)
			continue
		}

		item, err := r.documentToDomain(&doc)
		if err != nil {
			r.logger.Warn("Failed to convert document to domain", "error", err)
			continue
		}

		items = append(items, item)
	}

	return items, nil
}

// FindAvailableItems retrieves items with available stock
func (r *MongoInventoryRepository) FindAvailableItems() ([]*domain.InventoryItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	filter := bson.M{
		"stock_level": bson.M{"$gt": 0},
		"status":      int(domain.ItemStatusActive),
	}

	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		r.logger.Error("Failed to find available items", "error", err)
		return nil, fmt.Errorf("failed to find available items: %w", err)
	}
	defer cursor.Close(ctx)

	var items []*domain.InventoryItem
	for cursor.Next(ctx) {
		var doc inventoryItemDoc
		if err := cursor.Decode(&doc); err != nil {
			r.logger.Warn("Failed to decode inventory item", "error", err)
			continue
		}

		item, err := r.documentToDomain(&doc)
		if err != nil {
			r.logger.Warn("Failed to convert document to domain", "error", err)
			continue
		}

		items = append(items, item)
	}

	return items, nil
}

// Delete removes an inventory item from the database
func (r *MongoInventoryRepository) Delete(id string) error {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	filter := bson.M{"item_id": id}
	
	result, err := r.collection.DeleteOne(ctx, filter)
	if err != nil {
		r.logger.Error("Failed to delete inventory item", "error", err, "itemID", id)
		return fmt.Errorf("failed to delete inventory item: %w", err)
	}

	if result.DeletedCount == 0 {
		return domain.ErrItemNotFound
	}

	r.logger.Info("Inventory item deleted", "itemID", id)
	return nil
}

// Search finds items by name, description, or SKU using text search
func (r *MongoInventoryRepository) Search(query string) ([]*domain.InventoryItem, error) {
	ctx, cancel := context.WithTimeout(context.Background(), r.timeout)
	defer cancel()

	// Use MongoDB text search if available, otherwise use regex
	var filter bson.M
	
	if query == "" {
		// Return all active items if no query
		filter = bson.M{"status": int(domain.ItemStatusActive)}
	} else {
		// Try text search first
		filter = bson.M{
			"$text": bson.M{"$search": query},
			"status": int(domain.ItemStatusActive),
		}
		
		// If text index doesn't exist, fall back to regex search
		testCtx, testCancel := context.WithTimeout(context.Background(), 1*time.Second)
		defer testCancel()
		
		testCursor, testErr := r.collection.Find(testCtx, filter, options.Find().SetLimit(1))
		if testErr != nil && strings.Contains(testErr.Error(), "text index") {
			// Text index doesn't exist, use regex search
			filter = bson.M{
				"$or": []bson.M{
					{"name": bson.M{"$regex": query, "$options": "i"}},
					{"description": bson.M{"$regex": query, "$options": "i"}},
					{"sku": bson.M{"$regex": query, "$options": "i"}},
				},
				"status": int(domain.ItemStatusActive),
			}
		}
		if testCursor != nil {
			testCursor.Close(testCtx)
		}
	}

	cursor, err := r.collection.Find(ctx, filter)
	if err != nil {
		r.logger.Error("Failed to search inventory items", "error", err, "query", query)
		return nil, fmt.Errorf("failed to search inventory items: %w", err)
	}
	defer cursor.Close(ctx)

	var items []*domain.InventoryItem
	for cursor.Next(ctx) {
		var doc inventoryItemDoc
		if err := cursor.Decode(&doc); err != nil {
			r.logger.Warn("Failed to decode inventory item", "error", err)
			continue
		}

		item, err := r.documentToDomain(&doc)
		if err != nil {
			r.logger.Warn("Failed to convert document to domain", "error", err)
			continue
		}

		items = append(items, item)
	}

	return items, nil
}

// Close closes the MongoDB connection
func (r *MongoInventoryRepository) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	
	return r.client.Disconnect(ctx)
}

// createIndexes creates MongoDB indexes for optimal query performance
func (r *MongoInventoryRepository) createIndexes(ctx context.Context) error {
	indexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "sku", Value: 1}},
			Options: options.Index().SetUnique(true).SetName(skuIndex),
		},
		{
			Keys:    bson.D{{Key: "category", Value: 1}},
			Options: options.Index().SetName(categoryIndex),
		},
		{
			Keys: bson.D{
				{Key: "stock_level", Value: 1},
				{Key: "status", Value: 1},
			},
			Options: options.Index().SetName(stockIndex),
		},
		{
			Keys:    bson.D{{Key: "status", Value: 1}},
			Options: options.Index().SetName(statusIndex),
		},
		{
			Keys: bson.D{
				{Key: "name", Value: "text"},
				{Key: "description", Value: "text"},
				{Key: "sku", Value: "text"},
			},
			Options: options.Index().SetName(textIndex),
		},
	}

	indexNames, err := r.collection.Indexes().CreateMany(ctx, indexes)
	if err != nil {
		return fmt.Errorf("failed to create indexes: %w", err)
	}

	r.logger.Info("Created MongoDB indexes", "indexes", indexNames)
	return nil
}

// Conversion methods between domain and MongoDB models

// domainToDocument converts a domain InventoryItem to MongoDB document
func (r *MongoInventoryRepository) domainToDocument(item *domain.InventoryItem) *inventoryItemDoc {
	// Convert reservations
	reservations := make([]reservationDoc, 0)
	for _, reservation := range item.GetActiveReservations() {
		reservations = append(reservations, reservationDoc{
			ID:         reservation.ID(),
			OrderID:    reservation.OrderID(),
			ItemID:     reservation.ItemID(),
			Quantity:   reservation.Quantity(),
			ReservedAt: reservation.ReservedAt(),
			ExpiresAt:  reservation.ExpiresAt(),
			Status:     int(reservation.Status()),
		})
	}

	return &inventoryItemDoc{
		ItemID:        item.ID(),
		SKU:           item.SKU(),
		Name:          item.Name(),
		Description:   item.Description(),
		Category:      int(item.Category()),
		StockLevel:    item.StockLevel(),
		ReservedStock: item.ReservedStock(),
		TotalStock:    item.TotalStock(),
		MinStockLevel: item.MinStockLevel(),
		MaxStockLevel: item.MaxStockLevel(),
		Reservations:  reservations,
		UnitPrice: moneyDoc{
			Amount:   item.UnitPrice().Amount,
			Currency: item.UnitPrice().Currency,
		},
		Weight: item.Weight(),
		Dimensions: dimensionsDoc{
			Length: item.Dimensions().Length,
			Width:  item.Dimensions().Width,
			Height: item.Dimensions().Height,
		},
		Specifications: item.Specifications(),
		CreatedAt:      item.CreatedAt(),
		UpdatedAt:      item.UpdatedAt(),
		Version:        item.Version(),
		Status:         int(item.Status()),
	}
}

// documentToDomain converts a MongoDB document to domain InventoryItem
func (r *MongoInventoryRepository) documentToDomain(doc *inventoryItemDoc) (*domain.InventoryItem, error) {
	// Use the reconstruction factory method to restore full state
	item, err := domain.ReconstructInventoryItem(
		doc.ItemID,
		doc.SKU,
		doc.Name,
		doc.Description,
		domain.ItemCategory(doc.Category),
		doc.StockLevel,
		doc.ReservedStock,
		doc.TotalStock,
		doc.MinStockLevel,
		doc.MaxStockLevel,
		domain.Money{
			Amount:   doc.UnitPrice.Amount,
			Currency: doc.UnitPrice.Currency,
		},
		doc.Weight,
		domain.Dimensions{
			Length: doc.Dimensions.Length,
			Width:  doc.Dimensions.Width,
			Height: doc.Dimensions.Height,
		},
		doc.Specifications,
		doc.CreatedAt,
		doc.UpdatedAt,
		doc.Version,
		domain.ItemStatus(doc.Status),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to reconstruct domain item: %w", err)
	}

	// Restore reservations
	for _, reservationDoc := range doc.Reservations {
		err := item.RestoreReservation(
			reservationDoc.ID,
			reservationDoc.OrderID,
			reservationDoc.Quantity,
			reservationDoc.ReservedAt,
			reservationDoc.ExpiresAt,
			domain.ReservationStatus(reservationDoc.Status),
		)
		if err != nil {
			r.logger.Warn("Failed to restore reservation", 
				"reservationID", reservationDoc.ID,
				"error", err)
			// Continue processing other reservations
		}
	}

	r.logger.Debug("Successfully restored inventory item from database",
		"itemID", doc.ItemID,
		"sku", doc.SKU,
		"stockLevel", doc.StockLevel,
		"reservations", len(doc.Reservations))

	return item, nil
}

// Health check method
func (r *MongoInventoryRepository) HealthCheck(ctx context.Context) error {
	return r.client.Ping(ctx, nil)
}

// GetStats returns repository statistics
func (r *MongoInventoryRepository) GetStats(ctx context.Context) (map[string]interface{}, error) {
	stats := make(map[string]interface{})
	
	// Count total items
	totalCount, err := r.collection.CountDocuments(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	stats["total_items"] = totalCount
	
	// Count active items
	activeCount, err := r.collection.CountDocuments(ctx, bson.M{
		"status": int(domain.ItemStatusActive),
	})
	if err != nil {
		return nil, err
	}
	stats["active_items"] = activeCount
	
	// Count out of stock items
	outOfStockCount, err := r.collection.CountDocuments(ctx, bson.M{
		"stock_level": 0,
	})
	if err != nil {
		return nil, err
	}
	stats["out_of_stock_items"] = outOfStockCount
	
	return stats, nil
}