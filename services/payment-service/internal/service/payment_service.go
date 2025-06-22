package service

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/amiosamu/rocket-science/services/payment-service/internal/config"
	"github.com/amiosamu/rocket-science/services/payment-service/internal/domain"
)

// PaymentService defines the interface for payment operations
// This interface abstracts the business logic from the transport layer
type PaymentService interface {
	// ProcessPayment handles payment processing with business rules
	ProcessPayment(ctx context.Context, req ProcessPaymentRequest) (*ProcessPaymentResult, error)
	
	// GetPaymentStatus retrieves payment information
	GetPaymentStatus(ctx context.Context, req GetPaymentStatusRequest) (*GetPaymentStatusResult, error)
	
	// RefundPayment processes payment refunds
	RefundPayment(ctx context.Context, req RefundPaymentRequest) (*RefundPaymentResult, error)
	
	// GetPaymentsByOrderID retrieves all payments for an order
	GetPaymentsByOrderID(ctx context.Context, orderID string) ([]*domain.Payment, error)
}

// Service DTOs - Data Transfer Objects for the service layer
// These are different from both protobuf messages and domain objects
// They represent the service layer's view of the data

type ProcessPaymentRequest struct {
	OrderID       string
	UserID        string
	Amount        float64
	Currency      string
	PaymentMethod PaymentMethodDTO
	Description   string
}

type ProcessPaymentResult struct {
	Success       bool
	TransactionID string
	Message       string
	Status        string
	ProcessedAt   time.Time
	Amount        float64
	Currency      string
}

type GetPaymentStatusRequest struct {
	TransactionID string
	OrderID       string
}

type GetPaymentStatusResult struct {
	Found         bool
	TransactionID string
	OrderID       string
	Status        string
	Amount        float64
	Currency      string
	CreatedAt     time.Time
	ProcessedAt   *time.Time
	Message       string
}

type RefundPaymentRequest struct {
	TransactionID string
	Amount        float64
	Reason        string
	RequestedBy   string
}

type RefundPaymentResult struct {
	Success               bool
	RefundID              string
	OriginalTransactionID string
	RefundedAmount        float64
	Message               string
	ProcessedAt           time.Time
}

type PaymentMethodDTO struct {
	Type            string
	CreditCard      *CreditCardDTO
	BankTransfer    *BankTransferDTO
	DigitalWallet   *DigitalWalletDTO
}

type CreditCardDTO struct {
	MaskedNumber   string
	ExpiryMonth    string
	ExpiryYear     string
	CardholderName string
	Brand          string
}

type BankTransferDTO struct {
	BankName       string
	AccountNumber  string
	RoutingNumber  string
	AccountHolder  string
}

type DigitalWalletDTO struct {
	Provider  string
	WalletID  string
	Email     string
}

// paymentService is the concrete implementation of PaymentService
type paymentService struct {
	config     *config.Config
	logger     *slog.Logger
	repository PaymentRepository  // We'll implement this as in-memory for now
}

// PaymentRepository interface for payment persistence
// Since payment service has no database, we'll use in-memory storage
type PaymentRepository interface {
	Save(payment *domain.Payment) error
	FindByID(id string) (*domain.Payment, error)
	FindByTransactionID(transactionID string) (*domain.Payment, error)
	FindByOrderID(orderID string) ([]*domain.Payment, error)
}

// NewPaymentService creates a new payment service with dependencies
func NewPaymentService(cfg *config.Config, logger *slog.Logger) PaymentService {
	return &paymentService{
		config:     cfg,
		logger:     logger,
		repository: NewInMemoryPaymentRepository(), // In-memory implementation
	}
}

// ProcessPayment implements the main payment processing workflow
func (s *paymentService) ProcessPayment(ctx context.Context, req ProcessPaymentRequest) (*ProcessPaymentResult, error) {
	s.logger.Info("Processing payment",
		"orderID", req.OrderID,
		"userID", req.UserID,
		"amount", req.Amount,
		"currency", req.Currency)

	// Validate request
	if err := s.validateProcessPaymentRequest(req); err != nil {
		s.logger.Error("Invalid payment request", "error", err)
		return &ProcessPaymentResult{
			Success: false,
			Message: fmt.Sprintf("Invalid request: %v", err),
			Status:  "failed",
		}, nil // Return nil error since this is a business validation failure
	}

	// Convert DTO to domain objects
	money := domain.Money{
		Amount:   req.Amount,
		Currency: req.Currency,
	}

	paymentMethod, err := s.convertPaymentMethodToDomain(req.PaymentMethod)
	if err != nil {
		s.logger.Error("Invalid payment method", "error", err)
		return &ProcessPaymentResult{
			Success: false,
			Message: fmt.Sprintf("Invalid payment method: %v", err),
			Status:  "failed",
		}, nil
	}

	// Create domain payment object
	payment, err := domain.NewPayment(req.OrderID, req.UserID, money, paymentMethod, req.Description)
	if err != nil {
		s.logger.Error("Failed to create payment", "error", err)
		return &ProcessPaymentResult{
			Success: false,
			Message: fmt.Sprintf("Failed to create payment: %v", err),
			Status:  "failed",
		}, nil
	}

	// Save the payment in pending state
	if err := s.repository.Save(payment); err != nil {
		s.logger.Error("Failed to save payment", "error", err)
		return nil, fmt.Errorf("failed to save payment: %w", err)
	}

	// Process the payment using domain logic with configuration
	processingTime := s.config.Payment.ProcessingTimeMs
	successRate := s.config.Payment.SuccessRate

	s.logger.Info("Starting payment processing",
		"transactionID", payment.TransactionID(),
		"processingTimeMs", processingTime,
		"successRate", successRate)

	// This is where the business logic happens
	err = payment.Process(processingTime, successRate)

	// Update the payment state after processing
	if err := s.repository.Save(payment); err != nil {
		s.logger.Error("Failed to update payment after processing", "error", err)
		return nil, fmt.Errorf("failed to update payment: %w", err)
	}

	// Log the result
	if payment.IsCompleted() {
		s.logger.Info("Payment processed successfully",
			"transactionID", payment.TransactionID(),
			"amount", payment.Amount().String())
	} else {
		s.logger.Warn("Payment processing failed",
			"transactionID", payment.TransactionID(),
			"reason", payment.Message())
	}

	// Convert domain result back to service DTO
	return s.convertPaymentToProcessResult(payment), nil
}

// GetPaymentStatus retrieves payment status information
func (s *paymentService) GetPaymentStatus(ctx context.Context, req GetPaymentStatusRequest) (*GetPaymentStatusResult, error) {
	s.logger.Info("Getting payment status",
		"transactionID", req.TransactionID,
		"orderID", req.OrderID)

	var payment *domain.Payment
	var err error

	// Find payment by transaction ID or order ID
	if req.TransactionID != "" {
		payment, err = s.repository.FindByTransactionID(req.TransactionID)
	} else if req.OrderID != "" {
		payments, findErr := s.repository.FindByOrderID(req.OrderID)
		if findErr != nil {
			err = findErr
		} else if len(payments) > 0 {
			payment = payments[0] // Return the first payment for the order
		}
	} else {
		return &GetPaymentStatusResult{Found: false}, nil
	}

	if err != nil {
		s.logger.Error("Error finding payment", "error", err)
		return nil, fmt.Errorf("failed to find payment: %w", err)
	}

	if payment == nil {
		s.logger.Info("Payment not found",
			"transactionID", req.TransactionID,
			"orderID", req.OrderID)
		return &GetPaymentStatusResult{Found: false}, nil
	}

	// Convert domain object to service DTO
	return s.convertPaymentToStatusResult(payment), nil
}

// RefundPayment processes payment refunds
func (s *paymentService) RefundPayment(ctx context.Context, req RefundPaymentRequest) (*RefundPaymentResult, error) {
	s.logger.Info("Processing refund",
		"transactionID", req.TransactionID,
		"amount", req.Amount,
		"reason", req.Reason)

	// Find the original payment
	payment, err := s.repository.FindByTransactionID(req.TransactionID)
	if err != nil {
		s.logger.Error("Error finding payment for refund", "error", err)
		return nil, fmt.Errorf("failed to find payment: %w", err)
	}

	if payment == nil {
		return &RefundPaymentResult{
			Success: false,
			Message: "Payment not found",
		}, nil
	}

	// Create refund money object
	refundMoney := domain.Money{
		Amount:   req.Amount,
		Currency: payment.Amount().Currency,
	}

	// Process refund using domain logic
	err = payment.Refund(refundMoney, req.Reason)
	if err != nil {
		s.logger.Warn("Refund failed", "error", err, "transactionID", req.TransactionID)
		return &RefundPaymentResult{
			Success: false,
			Message: err.Error(),
		}, nil
	}

	// Save updated payment
	if err := s.repository.Save(payment); err != nil {
		s.logger.Error("Failed to save refunded payment", "error", err)
		return nil, fmt.Errorf("failed to save refunded payment: %w", err)
	}

	s.logger.Info("Refund processed successfully",
		"transactionID", req.TransactionID,
		"refundAmount", req.Amount)

	// Generate refund ID (in real systems, this might be from payment processor)
	refundID := fmt.Sprintf("ref_%d_%s", time.Now().Unix(), payment.TransactionID()[:8])

	return &RefundPaymentResult{
		Success:               true,
		RefundID:              refundID,
		OriginalTransactionID: payment.TransactionID(),
		RefundedAmount:        req.Amount,
		Message:               "Refund processed successfully",
		ProcessedAt:           time.Now(),
	}, nil
}

// GetPaymentsByOrderID retrieves all payments for a specific order
func (s *paymentService) GetPaymentsByOrderID(ctx context.Context, orderID string) ([]*domain.Payment, error) {
	s.logger.Info("Getting payments for order", "orderID", orderID)
	
	payments, err := s.repository.FindByOrderID(orderID)
	if err != nil {
		s.logger.Error("Error finding payments by order ID", "error", err)
		return nil, fmt.Errorf("failed to find payments: %w", err)
	}

	return payments, nil
}

// Validation methods

func (s *paymentService) validateProcessPaymentRequest(req ProcessPaymentRequest) error {
	if req.OrderID == "" {
		return fmt.Errorf("order ID is required")
	}
	if req.UserID == "" {
		return fmt.Errorf("user ID is required")
	}
	if req.Amount <= 0 {
		return fmt.Errorf("amount must be positive")
	}
	if req.Amount > s.config.Payment.MaxAmount {
		return fmt.Errorf("amount exceeds maximum allowed: %.2f", s.config.Payment.MaxAmount)
	}
	if req.Currency == "" {
		return fmt.Errorf("currency is required")
	}
	if len(req.Currency) != 3 {
		return fmt.Errorf("currency must be 3 characters")
	}
	return nil
}

// Conversion methods between DTOs and domain objects

func (s *paymentService) convertPaymentMethodToDomain(dto PaymentMethodDTO) (domain.PaymentMethod, error) {
	switch dto.Type {
	case "credit_card":
		if dto.CreditCard == nil {
			return domain.PaymentMethod{}, fmt.Errorf("credit card details required")
		}
		return domain.PaymentMethod{
			Type: domain.PaymentMethodCreditCard,
			Details: domain.CreditCardDetails{
				MaskedNumber:   dto.CreditCard.MaskedNumber,
				ExpiryMonth:    dto.CreditCard.ExpiryMonth,
				ExpiryYear:     dto.CreditCard.ExpiryYear,
				CardholderName: dto.CreditCard.CardholderName,
				Brand:          dto.CreditCard.Brand,
			},
		}, nil

	case "bank_transfer":
		if dto.BankTransfer == nil {
			return domain.PaymentMethod{}, fmt.Errorf("bank transfer details required")
		}
		return domain.PaymentMethod{
			Type: domain.PaymentMethodBankTransfer,
			Details: domain.BankTransferDetails{
				BankName:       dto.BankTransfer.BankName,
				AccountNumber:  dto.BankTransfer.AccountNumber,
				RoutingNumber:  dto.BankTransfer.RoutingNumber,
				AccountHolder:  dto.BankTransfer.AccountHolder,
			},
		}, nil

	case "digital_wallet":
		if dto.DigitalWallet == nil {
			return domain.PaymentMethod{}, fmt.Errorf("digital wallet details required")
		}
		return domain.PaymentMethod{
			Type: domain.PaymentMethodDigitalWallet,
			Details: domain.DigitalWalletDetails{
				Provider:  dto.DigitalWallet.Provider,
				WalletID:  dto.DigitalWallet.WalletID,
				Email:     dto.DigitalWallet.Email,
			},
		}, nil

	default:
		return domain.PaymentMethod{}, fmt.Errorf("unsupported payment method type: %s", dto.Type)
	}
}

func (s *paymentService) convertPaymentToProcessResult(payment *domain.Payment) *ProcessPaymentResult {
	var processedAt time.Time
	if payment.ProcessedAt() != nil {
		processedAt = *payment.ProcessedAt()
	} else {
		processedAt = time.Now()
	}

	return &ProcessPaymentResult{
		Success:       payment.IsCompleted(),
		TransactionID: payment.TransactionID(),
		Message:       payment.Message(),
		Status:        payment.Status().String(),
		ProcessedAt:   processedAt,
		Amount:        payment.Amount().Amount,
		Currency:      payment.Amount().Currency,
	}
}

func (s *paymentService) convertPaymentToStatusResult(payment *domain.Payment) *GetPaymentStatusResult {
	return &GetPaymentStatusResult{
		Found:         true,
		TransactionID: payment.TransactionID(),
		OrderID:       payment.OrderID(),
		Status:        payment.Status().String(),
		Amount:        payment.Amount().Amount,
		Currency:      payment.Amount().Currency,
		CreatedAt:     payment.CreatedAt(),
		ProcessedAt:   payment.ProcessedAt(),
		Message:       payment.Message(),
	}
}

// In-Memory Repository Implementation
// Since Payment Service has no database according to requirements

type inMemoryPaymentRepository struct {
	payments map[string]*domain.Payment
	mutex    sync.RWMutex
}

func NewInMemoryPaymentRepository() PaymentRepository {
	return &inMemoryPaymentRepository{
		payments: make(map[string]*domain.Payment),
	}
}

func (r *inMemoryPaymentRepository) Save(payment *domain.Payment) error {
	r.mutex.Lock()
	defer r.mutex.Unlock()
	
	// Store by transaction ID for easy lookup
	r.payments[payment.TransactionID()] = payment
	return nil
}

func (r *inMemoryPaymentRepository) FindByID(id string) (*domain.Payment, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	
	for _, payment := range r.payments {
		if payment.ID() == id {
			return payment, nil
		}
	}
	return nil, nil
}

func (r *inMemoryPaymentRepository) FindByTransactionID(transactionID string) (*domain.Payment, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	
	payment, exists := r.payments[transactionID]
	if !exists {
		return nil, nil
	}
	return payment, nil
}

func (r *inMemoryPaymentRepository) FindByOrderID(orderID string) ([]*domain.Payment, error) {
	r.mutex.RLock()
	defer r.mutex.RUnlock()
	
	var result []*domain.Payment
	for _, payment := range r.payments {
		if payment.OrderID() == orderID {
			result = append(result, payment)
		}
	}
	return result, nil
}