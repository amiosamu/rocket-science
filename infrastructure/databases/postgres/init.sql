-- =================================================================
-- PostgreSQL Database Initialization for Rocket Science Platform
-- =================================================================
-- This script initializes the PostgreSQL databases for IAM and Order services
-- It creates databases, users, extensions, and basic configurations

\echo 'Starting PostgreSQL initialization for Rocket Science Platform...'

-- =================================================================
-- Create Databases
-- =================================================================

-- Create IAM database
SELECT 'CREATE DATABASE rocket_iam' 
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'rocket_iam')\gexec

-- Create Orders database  
SELECT 'CREATE DATABASE rocket_orders'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'rocket_orders')\gexec

-- Create a general database (referenced in main docker-compose)
SELECT 'CREATE DATABASE rocket_science'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'rocket_science')\gexec

\echo 'Databases created successfully'

-- =================================================================
-- Create Users and Grant Permissions
-- =================================================================

-- Create rocket_user if not exists
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_catalog.pg_user WHERE usename = 'rocket_user') THEN
        CREATE USER rocket_user WITH PASSWORD 'rocket_password';
        \echo 'User rocket_user created'
    ELSE
        \echo 'User rocket_user already exists'
    END IF;
END
$$;

-- Grant database-level permissions
GRANT ALL PRIVILEGES ON DATABASE rocket_iam TO rocket_user;
GRANT ALL PRIVILEGES ON DATABASE rocket_orders TO rocket_user;
GRANT ALL PRIVILEGES ON DATABASE rocket_science TO rocket_user;

\echo 'Database permissions granted'

-- =================================================================
-- Configure IAM Database
-- =================================================================

\c rocket_iam;

\echo 'Configuring IAM database...'

-- Create extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Grant schema permissions
GRANT ALL PRIVILEGES ON SCHEMA public TO rocket_user;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO rocket_user;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO rocket_user;
GRANT ALL PRIVILEGES ON ALL FUNCTIONS IN SCHEMA public TO rocket_user;

-- Set default privileges for future objects
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL PRIVILEGES ON TABLES TO rocket_user;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL PRIVILEGES ON SEQUENCES TO rocket_user;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL PRIVILEGES ON FUNCTIONS TO rocket_user;

\echo 'IAM database configuration completed'

-- =================================================================
-- Configure Orders Database  
-- =================================================================

\c rocket_orders;

\echo 'Configuring Orders database...'

-- Create extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Grant schema permissions
GRANT ALL PRIVILEGES ON SCHEMA public TO rocket_user;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO rocket_user;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO rocket_user;
GRANT ALL PRIVILEGES ON ALL FUNCTIONS IN SCHEMA public TO rocket_user;

-- Set default privileges for future objects
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL PRIVILEGES ON TABLES TO rocket_user;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL PRIVILEGES ON SEQUENCES TO rocket_user;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL PRIVILEGES ON FUNCTIONS TO rocket_user;

\echo 'Orders database configuration completed'

-- =================================================================
-- Configure General Database (for cross-service operations)
-- =================================================================

\c rocket_science;

\echo 'Configuring general database...'

-- Create extensions
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Grant schema permissions
GRANT ALL PRIVILEGES ON SCHEMA public TO rocket_user;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO rocket_user;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO rocket_user;
GRANT ALL PRIVILEGES ON ALL FUNCTIONS IN SCHEMA public TO rocket_user;

-- Set default privileges for future objects
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL PRIVILEGES ON TABLES TO rocket_user;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL PRIVILEGES ON SEQUENCES TO rocket_user;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL PRIVILEGES ON FUNCTIONS TO rocket_user;

-- =================================================================
-- Create Health Check Function
-- =================================================================

CREATE OR REPLACE FUNCTION health_check()
RETURNS TABLE(
    database_name text,
    status text,
    extensions text[]
) AS $$
BEGIN
    RETURN QUERY
    SELECT 
        current_database()::text,
        'healthy'::text,
        ARRAY(SELECT extname FROM pg_extension)::text[];
END;
$$ LANGUAGE plpgsql;

\echo 'General database configuration completed'

-- =================================================================
-- Final Verification
-- =================================================================

\c rocket_science;

-- Create a view to monitor all databases
CREATE OR REPLACE VIEW database_status AS
SELECT 
    datname as database_name,
    pg_size_pretty(pg_database_size(datname)) as size,
    (SELECT count(*) FROM pg_stat_activity WHERE datname = d.datname) as active_connections
FROM pg_database d
WHERE datname IN ('rocket_iam', 'rocket_orders', 'rocket_science')
ORDER BY datname;

-- Grant access to the view
GRANT SELECT ON database_status TO rocket_user;

\echo ''
\echo '============================================================='
\echo 'PostgreSQL initialization completed successfully!'
\echo '============================================================='
\echo 'Created databases:'
\echo '  - rocket_iam (for IAM Service)'
\echo '  - rocket_orders (for Order Service)'  
\echo '  - rocket_science (for general operations)'
\echo ''
\echo 'Created user: rocket_user with full privileges'
\echo 'Installed extensions: uuid-ossp, pgcrypto'
\echo ''
\echo 'Next steps:'
\echo '  1. IAM Service will run its migrations to create users/sessions tables'
\echo '  2. Order Service will run its migrations to create orders/order_items tables'
\echo '  3. Services can connect using: host=postgres user=rocket_user dbname=rocket_*'
\echo '============================================================='
\echo ''
