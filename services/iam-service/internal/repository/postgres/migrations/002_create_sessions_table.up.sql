-- Create sessions table for audit trail and backup
-- Note: Active sessions are primarily stored in Redis for performance
-- This table serves as audit trail and backup storage
CREATE TABLE IF NOT EXISTS sessions (
    id UUID PRIMARY KEY,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    access_token_hash VARCHAR(255) NOT NULL, -- Hashed access token for security
    refresh_token_hash VARCHAR(255) NOT NULL, -- Hashed refresh token for security
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    created_at TIMESTAMP WITH TIME ZONE NOT NULL,
    expires_at TIMESTAMP WITH TIME ZONE NOT NULL,
    last_accessed_at TIMESTAMP WITH TIME ZONE NOT NULL,
    revoked_at TIMESTAMP WITH TIME ZONE,
    
    -- Security tracking
    ip_address INET,
    user_agent TEXT,
    device_fingerprint VARCHAR(255),
    
    -- Geographic information (optional)
    country_code VARCHAR(2),
    city VARCHAR(100),
    
    -- Session metadata
    metadata JSONB DEFAULT '{}'::jsonb,
    
    -- Constraints
    CONSTRAINT sessions_status_check CHECK (status IN ('active', 'expired', 'revoked', 'invalid'))
);

-- Create indexes for performance
CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_status ON sessions(status);
CREATE INDEX IF NOT EXISTS idx_sessions_created_at ON sessions(created_at);
CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);
CREATE INDEX IF NOT EXISTS idx_sessions_last_accessed_at ON sessions(last_accessed_at);
CREATE INDEX IF NOT EXISTS idx_sessions_ip_address ON sessions(ip_address);

-- Partial indexes for active sessions
CREATE INDEX IF NOT EXISTS idx_sessions_active ON sessions(user_id, expires_at) WHERE status = 'active';
CREATE INDEX IF NOT EXISTS idx_sessions_expired ON sessions(expires_at) WHERE status = 'expired';

-- Create a function to automatically update the last_accessed_at timestamp
CREATE OR REPLACE FUNCTION update_session_last_accessed()
RETURNS TRIGGER AS $$
BEGIN
    NEW.last_accessed_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- Create trigger to automatically update last_accessed_at on access token hash updates
-- (This indicates session access)
CREATE TRIGGER trigger_update_sessions_last_accessed
    BEFORE UPDATE OF access_token_hash ON sessions
    FOR EACH ROW
    EXECUTE FUNCTION update_session_last_accessed();

-- Create session cleanup function
CREATE OR REPLACE FUNCTION cleanup_expired_sessions()
RETURNS INTEGER AS $$
DECLARE
    cleaned_count INTEGER;
BEGIN
    UPDATE sessions 
    SET status = 'expired', revoked_at = NOW()
    WHERE status = 'active' 
    AND expires_at < NOW();
    
    GET DIAGNOSTICS cleaned_count = ROW_COUNT;
    RETURN cleaned_count;
END;
$$ LANGUAGE plpgsql;

-- Create session statistics view
CREATE OR REPLACE VIEW session_stats AS
SELECT 
    COUNT(*) as total_sessions,
    COUNT(*) FILTER (WHERE status = 'active' AND expires_at > NOW()) as active_sessions,
    COUNT(*) FILTER (WHERE status = 'expired') as expired_sessions,
    COUNT(*) FILTER (WHERE status = 'revoked') as revoked_sessions,
    COUNT(*) FILTER (WHERE created_at >= NOW() - INTERVAL '1 hour') as recent_sessions,
    COUNT(*) FILTER (WHERE created_at >= CURRENT_DATE) as today_sessions,
    COUNT(DISTINCT user_id) FILTER (WHERE status = 'active' AND expires_at > NOW()) as unique_active_users,
    COUNT(DISTINCT ip_address) as unique_ip_addresses
FROM sessions;

-- Create user session summary view
CREATE OR REPLACE VIEW user_session_summary AS
SELECT 
    u.id as user_id,
    u.email,
    u.first_name,
    u.last_name,
    COUNT(s.id) as total_sessions,
    COUNT(s.id) FILTER (WHERE s.status = 'active' AND s.expires_at > NOW()) as active_sessions,
    MAX(s.last_accessed_at) as last_session_access,
    COUNT(DISTINCT s.ip_address) as unique_ips
FROM users u
LEFT JOIN sessions s ON u.id = s.user_id
GROUP BY u.id, u.email, u.first_name, u.last_name;

-- Add comment for documentation
COMMENT ON TABLE sessions IS 'Session audit trail and backup storage. Active sessions are primarily managed in Redis.';
COMMENT ON COLUMN sessions.access_token_hash IS 'SHA-256 hash of access token for security';
COMMENT ON COLUMN sessions.refresh_token_hash IS 'SHA-256 hash of refresh token for security';
COMMENT ON COLUMN sessions.device_fingerprint IS 'Unique identifier for the device/browser';
COMMENT ON VIEW session_stats IS 'Real-time session statistics for monitoring';
COMMENT ON VIEW user_session_summary IS 'Per-user session summary for admin dashboard'; 