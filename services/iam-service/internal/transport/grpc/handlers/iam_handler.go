package handlers

import (
	"context"
	"fmt"
	"log"
	"strings"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/amiosamu/rocket-science/services/iam-service/internal/domain"
	"github.com/amiosamu/rocket-science/services/iam-service/internal/service"
	pb "github.com/amiosamu/rocket-science/services/iam-service/proto/iam"
)

// IAMHandler implements the gRPC IAMService
type IAMHandler struct {
	pb.UnimplementedIAMServiceServer
	authService *service.AuthService
	userService *service.UserService
}

// NewIAMHandler creates a new IAM gRPC handler
func NewIAMHandler(authService *service.AuthService, userService *service.UserService) *IAMHandler {
	return &IAMHandler{
		authService: authService,
		userService: userService,
	}
}

// Authentication Methods

// Login authenticates a user and creates a session
func (h *IAMHandler) Login(ctx context.Context, req *pb.LoginRequest) (*pb.LoginResponse, error) {
	log.Printf("Login attempt for email: %s", req.Email)

	if req.Email == "" || req.Password == "" {
		return nil, status.Error(codes.InvalidArgument, "email and password are required")
	}

	loginResp, err := h.authService.Login(ctx, req.Email, req.Password, req.IpAddress, req.UserAgent)
	if err != nil {
		log.Printf("Login failed for %s: %v", req.Email, err)
		if strings.Contains(err.Error(), "invalid credentials") {
			return nil, status.Error(codes.Unauthenticated, "invalid email or password")
		}
		if strings.Contains(err.Error(), "account locked") {
			return nil, status.Error(codes.PermissionDenied, "account is locked")
		}
		return nil, status.Error(codes.Internal, "login failed")
	}

	return &pb.LoginResponse{
		Success:      true,
		Message:      "Login successful",
		AccessToken:  loginResp.AccessToken,
		RefreshToken: loginResp.RefreshToken,
		SessionId:    loginResp.SessionID,
		User:         h.convertUserInfoToProto(loginResp.User),
		ExpiresAt:    timestamppb.New(loginResp.ExpiresAt),
	}, nil
}

// Logout invalidates a user session
func (h *IAMHandler) Logout(ctx context.Context, req *pb.LogoutRequest) (*pb.LogoutResponse, error) {
	if req.SessionId == "" {
		return nil, status.Error(codes.InvalidArgument, "session_id is required")
	}

	err := h.authService.Logout(ctx, req.SessionId)
	if err != nil {
		return nil, status.Error(codes.Internal, "logout failed")
	}

	return &pb.LogoutResponse{
		Success: true,
		Message: "Logout successful",
	}, nil
}

// RefreshToken refreshes an access token
func (h *IAMHandler) RefreshToken(ctx context.Context, req *pb.RefreshTokenRequest) (*pb.RefreshTokenResponse, error) {
	if req.RefreshToken == "" || req.SessionId == "" {
		return nil, status.Error(codes.InvalidArgument, "refresh_token and session_id are required")
	}

	refreshResp, err := h.authService.RefreshToken(ctx, req.SessionId, req.RefreshToken)
	if err != nil {
		return nil, status.Error(codes.Unauthenticated, "invalid refresh token")
	}

	return &pb.RefreshTokenResponse{
		Success:     true,
		Message:     "Token refreshed successfully",
		AccessToken: refreshResp.AccessToken,
		ExpiresAt:   timestamppb.New(refreshResp.ExpiresAt),
	}, nil
}

// Session Management Methods

// ValidateSession validates an access token
func (h *IAMHandler) ValidateSession(ctx context.Context, req *pb.ValidateSessionRequest) (*pb.ValidateSessionResponse, error) {
	if req.AccessToken == "" {
		return &pb.ValidateSessionResponse{
			Valid:   false,
			Message: "Access token required",
		}, nil
	}

	validateResp, err := h.authService.ValidateToken(ctx, req.AccessToken)
	if err != nil {
		return &pb.ValidateSessionResponse{
			Valid:   false,
			Message: "Invalid token",
		}, nil
	}

	return &pb.ValidateSessionResponse{
		Valid:   validateResp.Valid,
		Message: "Session is valid",
		User:    h.convertUserInfoToProto(validateResp.User),
		Session: h.convertSessionInfoToProto(validateResp.SessionInfo),
	}, nil
}

// GetSessionInfo retrieves session information
func (h *IAMHandler) GetSessionInfo(ctx context.Context, req *pb.GetSessionInfoRequest) (*pb.GetSessionInfoResponse, error) {
	if req.SessionId == "" {
		return nil, status.Error(codes.InvalidArgument, "session_id is required")
	}

	sessionInfo, userInfo, err := h.authService.GetSessionInfo(ctx, req.SessionId)
	if err != nil {
		return &pb.GetSessionInfoResponse{Found: false}, nil
	}

	return &pb.GetSessionInfoResponse{
		Found:   true,
		Session: h.convertSessionInfoToProto(sessionInfo),
		User:    h.convertUserInfoToProto(userInfo),
	}, nil
}

// InvalidateSession invalidates a session
func (h *IAMHandler) InvalidateSession(ctx context.Context, req *pb.InvalidateSessionRequest) (*pb.InvalidateSessionResponse, error) {
	if req.SessionId == "" {
		return nil, status.Error(codes.InvalidArgument, "session_id is required")
	}

	err := h.authService.RevokeSession(ctx, req.SessionId)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to invalidate session")
	}

	return &pb.InvalidateSessionResponse{
		Success: true,
		Message: "Session invalidated successfully",
	}, nil
}

// User Management Methods

// CreateUser creates a new user
func (h *IAMHandler) CreateUser(ctx context.Context, req *pb.CreateUserRequest) (*pb.CreateUserResponse, error) {
	if req.Email == "" || req.Password == "" {
		return nil, status.Error(codes.InvalidArgument, "email and password are required")
	}

	role, err := h.convertProtoRoleToDomain(req.Role)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "invalid role")
	}

	createReq := &service.CreateUserRequest{
		Email:     req.Email,
		Password:  req.Password,
		FirstName: req.FirstName,
		LastName:  req.LastName,
		Role:      role,
	}

	userInfo, err := h.userService.CreateUser(ctx, createReq)
	if err != nil {
		if strings.Contains(err.Error(), "email exists") {
			return nil, status.Error(codes.AlreadyExists, "email already exists")
		}
		return nil, status.Error(codes.Internal, "user creation failed")
	}

	return &pb.CreateUserResponse{
		Success: true,
		Message: "User created successfully",
		User:    h.convertUserInfoToProto(userInfo),
		UserId:  userInfo.ID,
	}, nil
}

// GetUser retrieves a user by ID or email
func (h *IAMHandler) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.GetUserResponse, error) {
	var user *service.UserInfo
	var err error

	switch identifier := req.Identifier.(type) {
	case *pb.GetUserRequest_UserId:
		user, err = h.userService.GetUser(ctx, identifier.UserId)
	case *pb.GetUserRequest_Email:
		user, err = h.userService.GetUserByEmail(ctx, identifier.Email)
	default:
		return nil, status.Error(codes.InvalidArgument, "user_id or email is required")
	}

	if err != nil {
		return &pb.GetUserResponse{
			Found:   false,
			Message: "User not found",
		}, nil
	}

	return &pb.GetUserResponse{
		Found:   true,
		User:    h.convertUserInfoToProto(user),
		Message: "User found",
	}, nil
}

// User Management Methods - Complete Implementations

func (h *IAMHandler) UpdateUser(ctx context.Context, req *pb.UpdateUserRequest) (*pb.UpdateUserResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	// Create update request
	updateReq := &service.UpdateUserRequest{}

	// Update fields if provided
	if req.FirstName != nil {
		updateReq.FirstName = req.FirstName
	}
	if req.LastName != nil {
		updateReq.LastName = req.LastName
	}
	if req.Role != nil {
		role, err := h.convertProtoRoleToDomain(*req.Role)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid role")
		}
		updateReq.Role = &role
	}
	if req.Status != nil {
		status_val := h.convertProtoStatusToDomain(*req.Status)
		updateReq.Status = &status_val
	}

	userInfo, err := h.userService.UpdateUser(ctx, req.UserId, updateReq)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, status.Error(codes.NotFound, "user not found")
		}
		if strings.Contains(err.Error(), "email exists") {
			return nil, status.Error(codes.AlreadyExists, "email already exists")
		}
		return nil, status.Error(codes.Internal, "failed to update user")
	}

	return &pb.UpdateUserResponse{
		Success: true,
		Message: "User updated successfully",
		User:    h.convertUserInfoToProto(userInfo),
	}, nil
}

func (h *IAMHandler) DeleteUser(ctx context.Context, req *pb.DeleteUserRequest) (*pb.DeleteUserResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	err := h.userService.DeleteUser(ctx, req.UserId)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, status.Error(codes.NotFound, "user not found")
		}
		return nil, status.Error(codes.Internal, "failed to delete user")
	}

	return &pb.DeleteUserResponse{
		Success: true,
		Message: "User deleted successfully",
	}, nil
}

func (h *IAMHandler) ListUsers(ctx context.Context, req *pb.ListUsersRequest) (*pb.ListUsersResponse, error) {
	// Create options
	options := service.UserListOptions{
		Limit:  int(req.Limit),
		Offset: int(req.Offset),
		Search: req.SearchQuery,
	}

	if req.RoleFilter != nil {
		role, err := h.convertProtoRoleToDomain(*req.RoleFilter)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, "invalid role filter")
		}
		options.Role = &role
	}

	if req.StatusFilter != nil {
		status_val := h.convertProtoStatusToDomain(*req.StatusFilter)
		options.Status = &status_val
	}

	// Set default limit if not provided
	if options.Limit <= 0 {
		options.Limit = 20
	}
	if options.Limit > 100 {
		options.Limit = 100 // Maximum limit
	}

	result, err := h.userService.ListUsers(ctx, options)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to list users")
	}

	// Convert users to proto
	var protoUsers []*pb.User
	for _, user := range result.Users {
		protoUsers = append(protoUsers, h.convertUserInfoToProto(user))
	}

	hasMore := options.Offset+options.Limit < result.Total

	return &pb.ListUsersResponse{
		Users:      protoUsers,
		TotalCount: int32(result.Total),
		HasMore:    hasMore,
	}, nil
}

// Profile Management Methods

func (h *IAMHandler) GetProfile(ctx context.Context, req *pb.GetProfileRequest) (*pb.GetProfileResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	userInfo, err := h.userService.GetUser(ctx, req.UserId)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, status.Error(codes.NotFound, "user not found")
		}
		return nil, status.Error(codes.Internal, "failed to get user profile")
	}

	profile := &pb.UserProfile{
		UserId:    userInfo.ID,
		FirstName: userInfo.FirstName,
		LastName:  userInfo.LastName,
		Email:     userInfo.Email,
		UpdatedAt: timestamppb.New(userInfo.UpdatedAt),
	}

	return &pb.GetProfileResponse{
		Found:   true,
		Profile: profile,
	}, nil
}

func (h *IAMHandler) UpdateProfile(ctx context.Context, req *pb.UpdateProfileRequest) (*pb.UpdateProfileResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	// Create update request with profile fields
	updateReq := &service.UpdateUserRequest{}

	if req.FirstName != nil {
		updateReq.FirstName = req.FirstName
	}
	if req.LastName != nil {
		updateReq.LastName = req.LastName
	}
	if req.Phone != nil {
		updateReq.Phone = req.Phone
	}
	if req.TelegramUsername != nil {
		updateReq.TelegramUsername = req.TelegramUsername
	}

	userInfo, err := h.userService.UpdateUser(ctx, req.UserId, updateReq)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, status.Error(codes.NotFound, "user not found")
		}
		return nil, status.Error(codes.Internal, "failed to update profile")
	}

	profile := &pb.UserProfile{
		UserId:    userInfo.ID,
		FirstName: userInfo.FirstName,
		LastName:  userInfo.LastName,
		Email:     userInfo.Email,
		UpdatedAt: timestamppb.New(userInfo.UpdatedAt),
	}

	return &pb.UpdateProfileResponse{
		Success: true,
		Message: "Profile updated successfully",
		Profile: profile,
	}, nil
}

func (h *IAMHandler) ChangePassword(ctx context.Context, req *pb.ChangePasswordRequest) (*pb.ChangePasswordResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}
	if req.CurrentPassword == "" {
		return nil, status.Error(codes.InvalidArgument, "current_password is required")
	}
	if req.NewPassword == "" {
		return nil, status.Error(codes.InvalidArgument, "new_password is required")
	}

	err := h.authService.ChangePassword(ctx, req.UserId, req.CurrentPassword, req.NewPassword, true, "")
	if err != nil {
		if strings.Contains(err.Error(), "invalid credentials") {
			return nil, status.Error(codes.Unauthenticated, "current password is incorrect")
		}
		if strings.Contains(err.Error(), "password") {
			return nil, status.Error(codes.InvalidArgument, "invalid password")
		}
		return nil, status.Error(codes.Internal, "failed to change password")
	}

	return &pb.ChangePasswordResponse{
		Success: true,
		Message: "Password changed successfully",
	}, nil
}

// Authorization and Permission Methods

func (h *IAMHandler) CheckPermission(ctx context.Context, req *pb.CheckPermissionRequest) (*pb.CheckPermissionResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}
	if req.Resource == "" {
		return nil, status.Error(codes.InvalidArgument, "resource is required")
	}
	if req.Action == "" {
		return nil, status.Error(codes.InvalidArgument, "action is required")
	}

	// Get user to check role
	userInfo, err := h.userService.GetUser(ctx, req.UserId)
	if err != nil {
		return nil, status.Error(codes.Internal, "failed to get user")
	}

	// Simple role-based permission check
	allowed := h.checkRolePermission(userInfo.Role, req.Resource, req.Action)
	permissions := h.getRolePermissions(userInfo.Role)

	return &pb.CheckPermissionResponse{
		Allowed:     allowed,
		Message:     h.getPermissionMessage(allowed, req.Resource, req.Action),
		Permissions: permissions,
	}, nil
}

func (h *IAMHandler) GetUserPermissions(ctx context.Context, req *pb.GetUserPermissionsRequest) (*pb.GetUserPermissionsResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	userInfo, err := h.userService.GetUser(ctx, req.UserId)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, status.Error(codes.NotFound, "user not found")
		}
		return nil, status.Error(codes.Internal, "failed to get user")
	}

	permissions := h.getRolePermissions(userInfo.Role)
	role := h.convertStringRoleToProto(userInfo.Role)

	return &pb.GetUserPermissionsResponse{
		Permissions: permissions,
		Role:        role,
	}, nil
}

// Telegram Integration Methods

func (h *IAMHandler) GetUserTelegramChatID(ctx context.Context, req *pb.GetUserTelegramChatIDRequest) (*pb.GetUserTelegramChatIDResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	chatID, username, err := h.userService.GetTelegramInfo(ctx, req.UserId)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return &pb.GetUserTelegramChatIDResponse{Found: false}, nil
		}
		return nil, status.Error(codes.Internal, "failed to get Telegram info")
	}

	return &pb.GetUserTelegramChatIDResponse{
		Found:            chatID != "",
		ChatId:           chatID,
		TelegramUsername: username,
	}, nil
}

func (h *IAMHandler) UpdateTelegramChatID(ctx context.Context, req *pb.UpdateTelegramChatIDRequest) (*pb.UpdateTelegramChatIDResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}
	if req.ChatId == "" {
		return nil, status.Error(codes.InvalidArgument, "chat_id is required")
	}

	err := h.userService.UpdateTelegramInfo(ctx, req.UserId, req.ChatId, req.TelegramUsername)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, status.Error(codes.NotFound, "user not found")
		}
		return nil, status.Error(codes.Internal, "failed to update Telegram info")
	}

	return &pb.UpdateTelegramChatIDResponse{
		Success: true,
		Message: "Telegram information updated successfully",
	}, nil
}

// Helper Methods for Conversion

// convertUserInfoToProto converts service UserInfo to protobuf User
func (h *IAMHandler) convertUserInfoToProto(user *service.UserInfo) *pb.User {
	if user == nil {
		return nil
	}

	return &pb.User{
		Id:        user.ID,
		Email:     user.Email,
		FirstName: user.FirstName,
		LastName:  user.LastName,
		Role:      h.convertStringRoleToProto(user.Role),
		Status:    h.convertStringStatusToProto(user.Status),
		CreatedAt: timestamppb.New(user.CreatedAt),
		UpdatedAt: timestamppb.New(user.UpdatedAt),
	}
}

// convertSessionInfoToProto converts domain SessionInfo to protobuf Session
func (h *IAMHandler) convertSessionInfoToProto(sessionInfo *domain.SessionInfo) *pb.Session {
	if sessionInfo == nil {
		return nil
	}

	return &pb.Session{
		Id:             sessionInfo.ID,
		UserId:         sessionInfo.UserID,
		CreatedAt:      timestamppb.New(sessionInfo.CreatedAt),
		ExpiresAt:      timestamppb.New(sessionInfo.ExpiresAt),
		LastAccessedAt: timestamppb.New(sessionInfo.LastAccessedAt),
		IpAddress:      sessionInfo.IPAddress,
		UserAgent:      sessionInfo.UserAgent,
		Status:         h.convertDomainSessionStatusToProto(sessionInfo.Status),
	}
}

// convertStringRoleToProto converts string role to protobuf UserRole
func (h *IAMHandler) convertStringRoleToProto(role string) pb.UserRole {
	switch role {
	case "customer":
		return pb.UserRole_USER_ROLE_CUSTOMER
	case "admin":
		return pb.UserRole_USER_ROLE_ADMIN
	case "operator":
		return pb.UserRole_USER_ROLE_OPERATOR
	case "support":
		return pb.UserRole_USER_ROLE_SUPPORT
	default:
		return pb.UserRole_USER_ROLE_UNSPECIFIED
	}
}

// convertStringStatusToProto converts string status to protobuf UserStatus
func (h *IAMHandler) convertStringStatusToProto(status string) pb.UserStatus {
	switch status {
	case "active":
		return pb.UserStatus_USER_STATUS_ACTIVE
	case "inactive":
		return pb.UserStatus_USER_STATUS_INACTIVE
	case "suspended":
		return pb.UserStatus_USER_STATUS_SUSPENDED
	case "deleted":
		return pb.UserStatus_USER_STATUS_DELETED
	default:
		return pb.UserStatus_USER_STATUS_UNSPECIFIED
	}
}

// convertProtoRoleToDomain converts protobuf UserRole to domain UserRole
func (h *IAMHandler) convertProtoRoleToDomain(role pb.UserRole) (domain.UserRole, error) {
	switch role {
	case pb.UserRole_USER_ROLE_CUSTOMER:
		return domain.RoleCustomer, nil
	case pb.UserRole_USER_ROLE_ADMIN:
		return domain.RoleAdmin, nil
	case pb.UserRole_USER_ROLE_OPERATOR:
		return domain.RoleOperator, nil
	case pb.UserRole_USER_ROLE_SUPPORT:
		return domain.RoleSupport, nil
	case pb.UserRole_USER_ROLE_UNSPECIFIED:
		return domain.RoleCustomer, nil
	default:
		return "", fmt.Errorf("unknown user role: %v", role)
	}
}

// convertDomainSessionStatusToProto converts domain SessionStatus to protobuf SessionStatus
func (h *IAMHandler) convertDomainSessionStatusToProto(status domain.SessionStatus) pb.SessionStatus {
	switch status {
	case domain.SessionStatusActive:
		return pb.SessionStatus_SESSION_STATUS_ACTIVE
	case domain.SessionStatusExpired:
		return pb.SessionStatus_SESSION_STATUS_EXPIRED
	case domain.SessionStatusRevoked:
		return pb.SessionStatus_SESSION_STATUS_REVOKED
	case domain.SessionStatusInvalid:
		return pb.SessionStatus_SESSION_STATUS_INVALID
	default:
		return pb.SessionStatus_SESSION_STATUS_UNSPECIFIED
	}
}

// convertProtoStatusToDomain converts protobuf UserStatus to domain UserStatus
func (h *IAMHandler) convertProtoStatusToDomain(status pb.UserStatus) domain.UserStatus {
	switch status {
	case pb.UserStatus_USER_STATUS_ACTIVE:
		return domain.StatusActive
	case pb.UserStatus_USER_STATUS_INACTIVE:
		return domain.StatusInactive
	case pb.UserStatus_USER_STATUS_SUSPENDED:
		return domain.StatusSuspended
	case pb.UserStatus_USER_STATUS_DELETED:
		return domain.StatusDeleted
	default:
		return domain.StatusActive
	}
}

// Permission helper methods

// checkRolePermission checks if a role has permission for a resource/action
func (h *IAMHandler) checkRolePermission(role, resource, action string) bool {
	permissions := h.getRolePermissions(role)

	// Check for wildcard permissions
	for _, perm := range permissions {
		if perm == "*" || perm == resource+".*" || perm == resource+"."+action {
			return true
		}
	}

	return false
}

// getRolePermissions returns the permissions for a role
func (h *IAMHandler) getRolePermissions(role string) []string {
	switch role {
	case "admin":
		return []string{
			"*", // Admin has all permissions
		}
	case "operator":
		return []string{
			"orders.*",
			"inventory.*",
			"users.read",
			"users.update",
		}
	case "support":
		return []string{
			"orders.read",
			"orders.update",
			"users.read",
			"inventory.read",
		}
	case "customer":
		return []string{
			"orders.create",
			"orders.read", // Own orders only
			"profile.*",
		}
	default:
		return []string{
			"profile.read",
		}
	}
}

// getPermissionMessage returns a message describing the permission check result
func (h *IAMHandler) getPermissionMessage(allowed bool, resource, action string) string {
	if allowed {
		return fmt.Sprintf("Permission granted for %s.%s", resource, action)
	}
	return fmt.Sprintf("Permission denied for %s.%s", resource, action)
}
