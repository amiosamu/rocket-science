// =================================================================
// MongoDB Database Initialization for Rocket Science Platform
// =================================================================
// This script initializes the MongoDB database for the Inventory Service
// It creates databases, users, collections, indexes, and seed data

print('Starting MongoDB initialization for rocket-science inventory service...');

// =================================================================
// Database and User Setup
// =================================================================

// Switch to the inventory database
db = db.getSiblingDB('inventory_db');

print('Creating inventory_db database...');

// Create user with read/write permissions
try {
    db.createUser({
        user: 'inventory_user',
        pwd: 'inventory_password',
        roles: [
            { role: 'readWrite', db: 'inventory_db' },
            { role: 'dbAdmin', db: 'inventory_db' }
        ]
    });
    print('✅ User inventory_user created successfully');
} catch (error) {
    if (error.code === 51003) { // User already exists
        print('⚠️  User inventory_user already exists');
    } else {
        print('❌ Error creating user:', error);
    }
}

// =================================================================
// Collections and Indexes Setup
// =================================================================

print('Setting up collections and indexes...');

// Create inventory_items collection
db.createCollection('inventory_items', {
    validator: {
        $jsonSchema: {
            bsonType: "object",
            required: ["item_id", "sku", "name", "category", "stock_level", "unit_price", "created_at"],
            properties: {
                item_id: {
                    bsonType: "string",
                    description: "Unique identifier for the inventory item"
                },
                sku: {
                    bsonType: "string",
                    description: "Stock Keeping Unit for the inventory item"
                },
                name: {
                    bsonType: "string",
                    description: "Human readable name of the item"
                },
                description: {
                    bsonType: "string",
                    description: "Detailed description of the item"
                },
                category: {
                    bsonType: "int",
                    description: "Category of the rocket component"
                },
                stock_level: {
                    bsonType: "int",
                    minimum: 0,
                    description: "Current stock level"
                },
                reserved_stock: {
                    bsonType: "int",
                    minimum: 0,
                    description: "Reserved stock level"
                },
                total_stock: {
                    bsonType: "int",
                    minimum: 0,
                    description: "Total stock level"
                },
                min_stock_level: {
                    bsonType: "int",
                    minimum: 0,
                    description: "Minimum stock level"
                },
                max_stock_level: {
                    bsonType: "int",
                    minimum: 0,
                    description: "Maximum stock level"
                },
                unit_price: {
                    bsonType: "object",
                    required: ["amount", "currency"],
                    properties: {
                        amount: {
                            bsonType: "double",
                            minimum: 0,
                            description: "Price amount in USD"
                        },
                        currency: {
                            bsonType: "string",
                            description: "Currency code"
                        }
                    }
                },
                weight: {
                    bsonType: "double",
                    minimum: 0,
                    description: "Weight in kilograms"
                },
                dimensions: {
                    bsonType: "object",
                    properties: {
                        length: { bsonType: "double", minimum: 0 },
                        width: { bsonType: "double", minimum: 0 },
                        height: { bsonType: "double", minimum: 0 }
                    }
                },
                specifications: {
                    bsonType: "object",
                    description: "Technical specifications"
                },
                created_at: {
                    bsonType: "date",
                    description: "Creation timestamp"
                },
                updated_at: {
                    bsonType: "date",
                    description: "Last update timestamp"
                },
                status: {
                    bsonType: "int",
                    description: "Status of the item"
                }
            }
        }
    }
});

print('✅ inventory_items collection created with validation schema');

// Create indexes for performance
print('Creating indexes...');

// Unique index on item_id
db.inventory_items.createIndex({ "item_id": 1 }, { unique: true, name: "idx_item_id_unique" });

// Category index for filtering
db.inventory_items.createIndex({ "category": 1 }, { name: "idx_category" });

// Stock level index for inventory management
db.inventory_items.createIndex({ "stock_level": 1 }, { name: "idx_stock_level" });

// Text search index for item search
db.inventory_items.createIndex(
    { 
        "name": "text", 
        "description": "text", 
        "item_id": "text" 
    }, 
    { 
        name: "idx_text_search",
        weights: { "name": 10, "item_id": 5, "description": 1 }
    }
);

// Compound index for category + stock level queries
db.inventory_items.createIndex({ "category": 1, "stock_level": 1 }, { name: "idx_category_stock" });

// Price range index
db.inventory_items.createIndex({ "unit_price.amount": 1 }, { name: "idx_unit_price" });

// Temporal indexes
db.inventory_items.createIndex({ "created_at": 1 }, { name: "idx_created_at" });
db.inventory_items.createIndex({ "updated_at": 1 }, { name: "idx_updated_at" });

print('✅ All indexes created successfully');

// =================================================================
// Seed Initial Rocket Components Data
// =================================================================

print('Seeding initial rocket components data...');

const currentTime = new Date();

const rocketComponents = [
    // Propulsion Systems
    {
        item_id: "engine-raptor-v1",
        name: "Raptor Engine V1",
        category: 1,
        description: "Full-flow staged combustion rocket engine using methane and oxygen",
        stock_level: 25,
        unit_price: {
            amount: 2500000.00,
            currency: "USD"
        },
        weight: 1600.0,
        dimensions: { length: 3.7, width: 1.3, height: 1.3 },
        specifications: {
            thrust_kn: 2200,
            specific_impulse_s: 330,
            fuel_type: "methane",
            oxidizer_type: "oxygen"
        },
        created_at: currentTime,
        updated_at: currentTime
    },
    {
        item_id: "engine-merlin-1d",
        name: "Merlin 1D Engine",
        category: 1, 
        description: "Gas-generator cycle rocket engine using RP-1 and oxygen",
        stock_level: 50,
        unit_price: {
            amount: 1000000.00,
            currency: "USD"
        },
        weight: 630.0,
        dimensions: { length: 3.0, width: 0.98, height: 0.98 },
        specifications: {
            thrust_kn: 845,
            specific_impulse_s: 282,
            fuel_type: "rp1",
            oxidizer_type: "oxygen"
        },
        created_at: currentTime,
        updated_at: currentTime
    },
    
    // Structure Components
    {
        item_id: "tank-fuel-main",
        name: "Main Fuel Tank",
        category: 2,
        description: "Primary fuel storage tank with integrated baffles",
        stock_level: 15,
        unit_price: {
            amount: 750000.00,
            currency: "USD"
        },
        weight: 2500.0,
        dimensions: { length: 50.0, width: 9.0, height: 9.0 },
        specifications: {
            capacity_liters: 125000,
            material: "carbon_fiber",
            pressure_rating_psi: 350
        },
        created_at: currentTime,
        updated_at: currentTime
    },
    {
        item_id: "tank-oxidizer-main", 
        name: "Main Oxidizer Tank",
        category: 2,
        description: "Primary oxidizer storage tank with thermal protection",
        stock_level: 12,
        unit_price: {
            amount: 850000.00,
            currency: "USD"
        },
        weight: 2800.0,
        dimensions: { length: 1200, width: 366, height: 366 },
        specifications: {
            capacity_liters: 150000,
            material: "stainless_steel",
            pressure_rating_psi: 400,
            cryogenic_capable: true
        },
        created_at: currentTime,
        updated_at: currentTime
    },
    
    // Electronics & Navigation
    {
        item_id: "guidance-computer-v3",
        name: "Flight Guidance Computer V3",
        category: 3,
        description: "Primary flight computer with redundant systems",
        stock_level: 30,
        unit_price: {
            amount: 500000.00,
            currency: "USD"
        },
        weight: 45.0,
        dimensions: { length: 0.5, width: 0.4, height: 0.2 },
        specifications: {
            processor: "radiation_hardened_arm",
            memory_gb: 64,
            storage_gb: 256,
            redundancy_level: "triple"
        },
        created_at: currentTime,
        updated_at: currentTime
    },
    {
        item_id: "nav-system-gps-ins",
        name: "GPS/INS Navigation System",
        category: 4,
        description: "Integrated GPS and inertial navigation system",
        stock_level: 40,
        unit_price: {
            amount: 300000.00,
            currency: "USD"
        },
        weight: 25.0,
        dimensions: { length: 0.3, width: 0.25, height: 0.15 },
        specifications: {
            accuracy_meters: 0.1,
            update_rate_hz: 100,
            mtbf_hours: 50000
        },
        created_at: currentTime,
        updated_at: currentTime
    },
    
    // Safety Systems
    {
        item_id: "abort-system-escape",
        name: "Launch Escape System",
        category: 5,
        description: "Emergency crew escape system with solid motors",
        stock_level: 8,
        unit_price: {
            amount: 2000000.00,
            currency: "USD"
        },
        weight: 6000.0,
        dimensions: { length: 0.8, width: 0.4, height: 0.4 },
        specifications: {
            escape_velocity_ms: 150,
            activation_time_ms: 40,
            motor_type: "solid_propellant"
        },
        created_at: currentTime,
        updated_at: currentTime
    },
    
    // Structure
    {
        item_id: "nosecone-composite",
        name: "Composite Nose Cone",
        category: 6,
        description: "Aerodynamic nose cone with integrated payload bay",
        stock_level: 20,
        unit_price: {
            amount: 400000.00,
            currency: "USD"
        },
        weight: 800.0,
        dimensions: { length: 600, width: 366, height: 366 },
        specifications: {
            material: "carbon_fiber_composite",
            payload_volume_m3: 15,
            max_dynamic_pressure_kpa: 35
        },
        created_at: currentTime,
        updated_at: currentTime
    },
    
    // Miscellaneous Critical Components
    {
        item_id: "parachute-main-drogue",
        name: "Main Drogue Parachute",
        category: 5,
        description: "Primary recovery parachute system",
        stock_level: 35,
        unit_price: {
            amount: 150000.00,
            currency: "USD"
        },
        weight: 120.0,
        dimensions: { length: 80, width: 60, height: 60 },
        specifications: {
            diameter_deployed_m: 30,
            max_deployment_speed_ms: 250,
            material: "ripstop_nylon"
        },
        created_at: currentTime,
        updated_at: currentTime
    },
    {
        item_id: "heat-shield-ceramic",
        name: "Ceramic Heat Shield Tiles",
        category: 6, 
        description: "Ultra-high temperature ceramic tiles for reentry protection",
        stock_level: 500,
        unit_price: {
            amount: 5000.00,
            currency: "USD"
        },
        weight: 2.5,
        dimensions: { length: 30, width: 30, height: 5 },
        specifications: {
            max_temperature_k: 1800,
            material: "ultra_high_temp_ceramic",
            thermal_conductivity: "low"
        },
        created_at: currentTime,
        updated_at: currentTime
    }
];

try {
    const insertResult = db.inventory_items.insertMany(rocketComponents);
    print(`✅ Successfully inserted ${insertResult.insertedIds.length} rocket components`);
} catch (error) {
    print('❌ Error inserting seed data:', error);
}

// =================================================================
// Create Operational Collections
// =================================================================

print('Creating operational collections...');

// Collection for inventory transactions (stock movements)
db.createCollection('inventory_transactions', {
    validator: {
        $jsonSchema: {
            bsonType: "object",
            required: ["transaction_id", "item_id", "transaction_type", "quantity", "timestamp"],
            properties: {
                transaction_id: { bsonType: "string" },
                item_id: { bsonType: "string" },
                transaction_type: { 
                    bsonType: "string",
                    enum: ["in", "out", "reserved", "unreserved", "adjustment"]
                },
                quantity: { bsonType: "int" },
                reference_id: { bsonType: "string" },
                reason: { bsonType: "string" },
                timestamp: { bsonType: "date" }
            }
        }
    }
});

// Indexes for inventory transactions
db.inventory_transactions.createIndex({ "item_id": 1, "timestamp": -1 });
db.inventory_transactions.createIndex({ "transaction_type": 1 });
db.inventory_transactions.createIndex({ "reference_id": 1 });

print('✅ inventory_transactions collection created');

// Collection for reservations
db.createCollection('inventory_reservations', {
    validator: {
        $jsonSchema: {
            bsonType: "object",
            required: ["reservation_id", "item_id", "quantity", "order_id", "expires_at"],
            properties: {
                reservation_id: { bsonType: "string" },
                item_id: { bsonType: "string" },
                quantity: { bsonType: "int", minimum: 1 },
                order_id: { bsonType: "string" },
                user_id: { bsonType: "string" },
                status: { 
                    bsonType: "string",
                    enum: ["active", "expired", "fulfilled", "cancelled"]
                },
                created_at: { bsonType: "date" },
                expires_at: { bsonType: "date" }
            }
        }
    }
});

// Indexes for reservations
db.inventory_reservations.createIndex({ "item_id": 1, "status": 1 });
db.inventory_reservations.createIndex({ "order_id": 1 });
db.inventory_reservations.createIndex({ "expires_at": 1 });

print('✅ inventory_reservations collection created');

// =================================================================
// Final Health Check and Statistics
// =================================================================

print('Running final health checks...');

// Count documents
const itemCount = db.inventory_items.countDocuments();
const transactionCount = db.inventory_transactions.countDocuments();
const reservationCount = db.inventory_reservations.countDocuments();

print('');
print('=============================================================');
print('MongoDB initialization completed successfully!');
print('=============================================================');
print('Database: inventory_db');
print('User: inventory_user (with readWrite + dbAdmin permissions)');
print('');
print('Collections created:');
print(`  - inventory_items (${itemCount} documents)`);
print(`  - inventory_transactions (${transactionCount} documents)`);  
print(`  - inventory_reservations (${reservationCount} documents)`);
print('');
print('Indexes created: 10+ performance-optimized indexes');
print('Validation schemas: Enforced for data integrity');
print('');
print('Seeded components by category:');
const categoryStats = db.inventory_items.aggregate([
    { $group: { _id: "$category", count: { $sum: 1 }, total_value: { $sum: "$unit_price.amount" } } },
    { $sort: { count: -1 } }
]).toArray();

categoryStats.forEach(stat => {
    print(`  - ${stat._id}: ${stat.count} items (Total value: $${stat.total_value.toLocaleString()})`);
});

print('');
print('Connection string for services:');
print('  mongodb://inventory_user:inventory_password@mongo:27017/inventory_db?authSource=inventory_db');
print('');
print('Next steps:');
print('  1. Inventory Service will connect and start serving requests');
print('  2. Use text search for component discovery: db.inventory_items.find({$text: {$search: "engine"}})');
print('  3. Monitor stock levels and reservations through the service APIs');
print('=============================================================');
print('');
