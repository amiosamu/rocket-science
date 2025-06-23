-- Database initialization script for IAM service
-- This script runs when the PostgreSQL container starts for the first time

-- Ensure the database exists
SELECT 'CREATE DATABASE iam_db'
WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'iam_db')\gexec

-- Connect to the iam_db database
\c iam_db;

-- Create extensions if needed
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "pgcrypto";

-- Create schemas
CREATE SCHEMA IF NOT EXISTS public;

-- Grant permissions
GRANT ALL PRIVILEGES ON DATABASE iam_db TO iam_user;
GRANT ALL PRIVILEGES ON SCHEMA public TO iam_user;
GRANT ALL PRIVILEGES ON ALL TABLES IN SCHEMA public TO iam_user;
GRANT ALL PRIVILEGES ON ALL SEQUENCES IN SCHEMA public TO iam_user;

-- Set default privileges for future objects
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL PRIVILEGES ON TABLES TO iam_user;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT ALL PRIVILEGES ON SEQUENCES TO iam_user;

-- Log completion
\echo 'Database initialization completed successfully!' 