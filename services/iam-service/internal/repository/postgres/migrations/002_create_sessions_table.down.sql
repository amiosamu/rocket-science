-- Drop views
DROP VIEW IF EXISTS user_session_summary;
DROP VIEW IF EXISTS session_stats;

-- Drop functions
DROP FUNCTION IF EXISTS cleanup_expired_sessions();
DROP FUNCTION IF EXISTS update_session_last_accessed();

-- Drop triggers (automatically dropped with functions)

-- Drop indexes (automatically dropped with table)

-- Drop table
DROP TABLE IF EXISTS sessions; 