-- Auth check Lua script for Envoy
-- This script validates session tokens with the IAM service

local json = require("json")

-- Helper function to extract session token from request
local function extract_session_token(request_handle)
    -- First, try Authorization header (Bearer token)
    local auth_header = request_handle:headers():get("authorization")
    if auth_header then
        local session_token = string.match(auth_header, "Bearer%s+(.+)")
        if session_token then
            return session_token
        end
    end
    
    -- If no Authorization header, try cookies
    local cookie_header = request_handle:headers():get("cookie")
    if cookie_header then
        local session_token = string.match(cookie_header, "session_token=([^;]+)")
        if session_token then
            return session_token
        end
    end
    
    return nil
end

-- Helper function to check if endpoint requires authentication
local function requires_auth(path, method)
    -- Public endpoints that don't require authentication
    local public_endpoints = {
        "/health",
        "/ready",
        "/metrics",
        "/iam.IAMService/Login",
        "/iam.IAMService/Register",
        "/iam.IAMService/RefreshToken"
    }
    
    for _, endpoint in ipairs(public_endpoints) do
        if string.find(path, endpoint) then
            return false
        end
    end
    
    return true
end

-- Helper function to validate session with IAM service
local function validate_session(request_handle, session_token)
    local headers, body = request_handle:httpCall(
        "iam-service",
        {
            [":method"] = "POST",
            [":path"] = "/validate-session",
            [":authority"] = "iam-service",
            ["content-type"] = "application/json"
        },
        '{"session_token":"' .. session_token .. '"}',
        5000 -- 5 second timeout
    )
    
    if not headers then
        request_handle:logErr("Failed to call IAM service for session validation")
        return nil
    end
    
    local status = headers[":status"]
    if status ~= "200" then
        request_handle:logWarn("Session validation failed with status: " .. (status or "unknown"))
        return nil
    end
    
    if body then
        local success, user_data = pcall(json.decode, body)
        if success and user_data then
            return user_data
        else
            request_handle:logErr("Failed to parse user data from IAM response")
        end
    end
    
    return nil
end

-- Main authentication function
function envoy_on_request(request_handle)
    local path = request_handle:headers():get(":path")
    local method = request_handle:headers():get(":method")
    
    -- Log the incoming request
    request_handle:logInfo("Processing request: " .. method .. " " .. path)
    
    -- Check if this endpoint requires authentication
    if not requires_auth(path, method) then
        request_handle:logInfo("Public endpoint, skipping authentication: " .. path)
        return
    end
    
    -- Extract session token
    local session_token = extract_session_token(request_handle)
    if not session_token then
        request_handle:logWarn("Missing session token for protected endpoint: " .. path)
        request_handle:respond(
            {
                [":status"] = "401",
                ["content-type"] = "application/json"
            },
            '{"error":"Unauthorized","message":"Missing session token"}'
        )
        return
    end
    
    -- Validate session with IAM service
    local user_data = validate_session(request_handle, session_token)
    if not user_data then
        request_handle:logWarn("Invalid session token for: " .. path)
        request_handle:respond(
            {
                [":status"] = "401",
                ["content-type"] = "application/json"
            },
            '{"error":"Unauthorized","message":"Invalid or expired session"}'
        )
        return
    end
    
    -- Add user information to headers for downstream services
    if user_data.user_id then
        request_handle:headers():add("x-user-id", tostring(user_data.user_id))
    end
    if user_data.email then
        request_handle:headers():add("x-user-email", user_data.email)
    end
    if user_data.role then
        request_handle:headers():add("x-user-role", user_data.role)
    end
    
    -- Add session token to headers for downstream services that might need it
    request_handle:headers():add("x-session-token", session_token)
    
    request_handle:logInfo("Authentication successful for user: " .. (user_data.email or user_data.user_id or "unknown"))
end

-- Function called on response (optional, for logging)
function envoy_on_response(response_handle)
    local status = response_handle:headers():get(":status")
    response_handle:logInfo("Response status: " .. (status or "unknown"))
end
