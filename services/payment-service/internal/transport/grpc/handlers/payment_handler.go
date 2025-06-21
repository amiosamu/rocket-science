package handlers

import (
	"context"
	"log/slog"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/amiosamu/rocket-science/services/payment-service/internal/service"
	pb "github.com/amiosamu/rocket-science/services/payment-service/proto/payment"
)

// PaymentHandler implements the PaymentServiceServer interface from protobuf
// It serves as the adapter between gRPC transport and our business service
type PaymentHandler struct {
	pb.UnimplementedPaymentServiceServer // Embedding for forward compatibility
	paymentService                       service.PaymentService
	logger                               *slog.Logger
}

// NewPaymentHandler creates a new gRPC payment handler
func NewPaymentHandler(paymentService service.PaymentService, logger *slog.Logger) *PaymentHandler {
	return &PaymentHandler{
		paymentService: paymentService,
		logger:         logger,
	}
}

// ProcessPayment handles payment processing requests via gRPC
func (h *PaymentHandler) ProcessPayment(ctx context.Context, req *pb.ProcessPaymentRequest) (*pb.ProcessPaymentResponse, error) {
	h.logger.Info("gRPC ProcessPayment called",
		"orderID", req.OrderId,
		"userID", req.UserId,
		"amount", req.Amount)

	// Validate required fields
	if err := h.validateProcessPaymentRequest(req); err != nil {
		h.logger.Warn("Invalid ProcessPayment request", "error", err)
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}

	// Convert protobuf request to service DTO
	serviceReq, err := h.convertToServiceProcessRequest(req)
	if err != nil {
		h.logger.Error("Failed to convert protobuf request", "error", err)
		return nil, status.Errorf(codes.InvalidArgument, "invalid request format: %v", err)
	}

	// Call business service
	result, err := h.paymentService.ProcessPayment(ctx, serviceReq)
	if err != nil {
		h.logger.Error("Payment service error", "error", err)
		return nil, status.Errorf(codes.Internal, "payment processing failed: %v", err)
	}

	// Convert service result to protobuf response
	response := h.convertToProcessPaymentResponse(result)

	h.logger.Info("ProcessPayment completed",
		"success", response.Success,
		"transactionID", response.TransactionId,
		"status", response.Status)

	return response, nil
}

// GetPaymentStatus handles payment status requests via gRPC
func (h *PaymentHandler) GetPaymentStatus(ctx context.Context, req *pb.GetPaymentStatusRequest) (*pb.GetPaymentStatusResponse, error) {
	h.logger.Info("gRPC GetPaymentStatus called",
		"transactionID", req.TransactionId,
		"orderID", req.OrderId)

	// Validate that at least one identifier is provided
	if req.TransactionId == "" && req.OrderId == "" {
		h.logger.Warn("GetPaymentStatus: no identifier provided")
		return nil, status.Errorf(codes.InvalidArgument, "either transaction_id or order_id must be provided")
	}

	// Convert to service request
	serviceReq := service.GetPaymentStatusRequest{
		TransactionID: req.TransactionId,
		OrderID:       req.OrderId,
	}

	// Call business service
	result, err := h.paymentService.GetPaymentStatus(ctx, serviceReq)
	if err != nil {
		h.logger.Error("Payment status service error", "error", err)
		return nil, status.Errorf(codes.Internal, "failed to get payment status: %v", err)
	}

	// Convert service result to protobuf response
	response := h.convertToGetPaymentStatusResponse(result)

	h.logger.Info("GetPaymentStatus completed", "found", response.Found)
	return response, nil
}

// RefundPayment handles refund requests via gRPC
func (h *PaymentHandler) RefundPayment(ctx context.Context, req *pb.RefundPaymentRequest) (*pb.RefundPaymentResponse, error) {
	h.logger.Info("gRPC RefundPayment called",
		"transactionID", req.TransactionId,
		"amount", req.Amount,
		"reason", req.Reason)

	// Validate request
	if err := h.validateRefundPaymentRequest(req); err != nil {
		h.logger.Warn("Invalid RefundPayment request", "error", err)
		return nil, status.Errorf(codes.InvalidArgument, "invalid request: %v", err)
	}

	// Convert to service request
	serviceReq := service.RefundPaymentRequest{
		TransactionID: req.TransactionId,
		Amount:        req.Amount,
		Reason:        req.Reason,
		RequestedBy:   req.RequestedBy,
	}

	// Call business service
	result, err := h.paymentService.RefundPayment(ctx, serviceReq)
	if err != nil {
		h.logger.Error("Refund service error", "error", err)
		return nil, status.Errorf(codes.Internal, "refund processing failed: %v", err)
	}

	// Convert service result to protobuf response
	response := h.convertToRefundPaymentResponse(result)

	h.logger.Info("RefundPayment completed",
		"success", response.Success,
		"refundID", response.RefundId)

	return response, nil
}

// Validation methods

func (h *PaymentHandler) validateProcessPaymentRequest(req *pb.ProcessPaymentRequest) error {
	if req.OrderId == "" {
		return status.Error(codes.InvalidArgument, "order_id is required")
	}
	if req.UserId == "" {
		return status.Error(codes.InvalidArgument, "user_id is required")
	}
	if req.Amount <= 0 {
		return status.Error(codes.InvalidArgument, "amount must be positive")
	}
	if req.Currency == "" {
		return status.Error(codes.InvalidArgument, "currency is required")
	}
	if req.PaymentMethod == nil {
		return status.Error(codes.InvalidArgument, "payment_method is required")
	}
	return nil
}

func (h *PaymentHandler) validateRefundPaymentRequest(req *pb.RefundPaymentRequest) error {
	if req.TransactionId == "" {
		return status.Error(codes.InvalidArgument, "transaction_id is required")
	}
	if req.Amount <= 0 {
		return status.Error(codes.InvalidArgument, "amount must be positive")
	}
	if req.Reason == "" {
		return status.Error(codes.InvalidArgument, "reason is required")
	}
	return nil
}

// Conversion methods: Protobuf -> Service DTOs

func (h *PaymentHandler) convertToServiceProcessRequest(req *pb.ProcessPaymentRequest) (service.ProcessPaymentRequest, error) {
	paymentMethod, err := h.convertPaymentMethodToService(req.PaymentMethod)
	if err != nil {
		return service.ProcessPaymentRequest{}, err
	}

	return service.ProcessPaymentRequest{
		OrderID:       req.OrderId,
		UserID:        req.UserId,
		Amount:        req.Amount,
		Currency:      req.Currency,
		PaymentMethod: paymentMethod,
		Description:   req.Description,
	}, nil
}

func (h *PaymentHandler) convertPaymentMethodToService(pm *pb.PaymentMethod) (service.PaymentMethodDTO, error) {
	switch pm.Type {
	case pb.PaymentType_PAYMENT_TYPE_CREDIT_CARD:
		if pm.CreditCard == nil {
			return service.PaymentMethodDTO{}, status.Error(codes.InvalidArgument, "credit_card details required")
		}
		return service.PaymentMethodDTO{
			Type: "credit_card",
			CreditCard: &service.CreditCardDTO{
				MaskedNumber:   pm.CreditCard.MaskedNumber,
				ExpiryMonth:    pm.CreditCard.ExpiryMonth,
				ExpiryYear:     pm.CreditCard.ExpiryYear,
				CardholderName: pm.CreditCard.CardholderName,
				Brand:          pm.CreditCard.Brand,
			},
		}, nil

	case pb.PaymentType_PAYMENT_TYPE_BANK_TRANSFER:
		if pm.BankTransfer == nil {
			return service.PaymentMethodDTO{}, status.Error(codes.InvalidArgument, "bank_transfer details required")
		}
		return service.PaymentMethodDTO{
			Type: "bank_transfer",
			BankTransfer: &service.BankTransferDTO{
				BankName:      pm.BankTransfer.BankName,
				AccountNumber: pm.BankTransfer.AccountNumber,
				RoutingNumber: pm.BankTransfer.RoutingNumber,
				AccountHolder: pm.BankTransfer.AccountHolder,
			},
		}, nil

	case pb.PaymentType_PAYMENT_TYPE_DIGITAL_WALLET:
		if pm.DigitalWallet == nil {
			return service.PaymentMethodDTO{}, status.Error(codes.InvalidArgument, "digital_wallet details required")
		}
		return service.PaymentMethodDTO{
			Type: "digital_wallet",
			DigitalWallet: &service.DigitalWalletDTO{
				Provider: pm.DigitalWallet.Provider,
				WalletID: pm.DigitalWallet.WalletId,
				Email:    pm.DigitalWallet.Email,
			},
		}, nil

	default:
		return service.PaymentMethodDTO{}, status.Errorf(codes.InvalidArgument, "unsupported payment method type: %v", pm.Type)
	}
}

// Conversion methods: Service DTOs -> Protobuf responses

func (h *PaymentHandler) convertToProcessPaymentResponse(result *service.ProcessPaymentResult) *pb.ProcessPaymentResponse {
	status := h.convertStatusToProto(result.Status)

	return &pb.ProcessPaymentResponse{
		Success:         result.Success,
		TransactionId:   result.TransactionID,
		Message:         result.Message,
		Status:          status,
		ProcessedAt:     timestamppb.New(result.ProcessedAt),
		ProcessedAmount: result.Amount,
		Currency:        result.Currency,
	}
}

func (h *PaymentHandler) convertToGetPaymentStatusResponse(result *service.GetPaymentStatusResult) *pb.GetPaymentStatusResponse {
	if !result.Found {
		return &pb.GetPaymentStatusResponse{
			Found: false,
		}
	}

	status := h.convertStatusToProto(result.Status)

	response := &pb.GetPaymentStatusResponse{
		Found:         true,
		TransactionId: result.TransactionID,
		OrderId:       result.OrderID,
		Status:        status,
		Amount:        result.Amount,
		Currency:      result.Currency,
		CreatedAt:     timestamppb.New(result.CreatedAt),
		Message:       result.Message,
	}

	if result.ProcessedAt != nil {
		response.ProcessedAt = timestamppb.New(*result.ProcessedAt)
	}

	return response
}

func (h *PaymentHandler) convertToRefundPaymentResponse(result *service.RefundPaymentResult) *pb.RefundPaymentResponse {
	return &pb.RefundPaymentResponse{
		Success:               result.Success,
		RefundId:              result.RefundID,
		OriginalTransactionId: result.OriginalTransactionID,
		RefundedAmount:        result.RefundedAmount,
		Message:               result.Message,
		ProcessedAt:           timestamppb.New(result.ProcessedAt),
	}
}

func (h *PaymentHandler) convertStatusToProto(statusStr string) pb.PaymentStatus {
	switch statusStr {
	case "pending":
		return pb.PaymentStatus_PAYMENT_STATUS_PENDING
	case "completed":
		return pb.PaymentStatus_PAYMENT_STATUS_COMPLETED
	case "failed":
		return pb.PaymentStatus_PAYMENT_STATUS_FAILED
	case "cancelled":
		return pb.PaymentStatus_PAYMENT_STATUS_CANCELLED
	case "refunded":
		return pb.PaymentStatus_PAYMENT_STATUS_REFUNDED
	case "partially_refunded":
		return pb.PaymentStatus_PAYMENT_STATUS_PARTIAL_REFUND
	default:
		return pb.PaymentStatus_PAYMENT_STATUS_UNSPECIFIED
	}
}
