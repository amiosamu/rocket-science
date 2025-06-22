package domain

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Payment represents the core payment entity in our domain
// This is our aggregate root - it encapsulates all payment-related business logic
type Payment struct {
	// Identity fields
	id            string    // Unique payment identifier
	transactionID string    // External transaction reference
	orderID       string    // Associated order identifier
	userID        string    // User who made the payment
	
	// Value objects
	amount        Money          // Payment amount with currency
	paymentMethod PaymentMethod  // How the payment was made
	
	// State tracking
	status        PaymentStatus  // Current payment status
	message       string         // Status message or error description
	
	// Audit fields
	createdAt     time.Time      // When payment was initiated
	processedAt   *time.Time     // When payment was completed (nil if not processed)
	
	// Business fields
	description   string         // Payment description
	metadata      map[string]string // Additional payment metadata
}

// PaymentStatus represents the lifecycle states of a payment
// Think of this as a state machine - payments move through these states
type PaymentStatus int

const (
	PaymentStatusPending PaymentStatus = iota  // Payment is being processed
	PaymentStatusCompleted                     // Payment completed successfully
	PaymentStatusFailed                        // Payment failed for some reason
	PaymentStatusCancelled                     // Payment was cancelled by user/system
	PaymentStatusRefunded                      // Payment was fully refunded
	PaymentStatusPartiallyRefunded             // Payment was partially refunded
)

// String provides human-readable status names
func (ps PaymentStatus) String() string {
	switch ps {
	case PaymentStatusPending:
		return "pending"
	case PaymentStatusCompleted:
		return "completed"
	case PaymentStatusFailed:
		return "failed"
	case PaymentStatusCancelled:
		return "cancelled"
	case PaymentStatusRefunded:
		return "refunded"
	case PaymentStatusPartiallyRefunded:
		return "partially_refunded"
	default:
		return "unknown"
	}
}

// Money is a value object that encapsulates amount and currency
// Value objects are immutable and compared by value, not identity
type Money struct {
	Amount   float64 // The monetary amount
	Currency string  // Currency code (e.g., "USD", "EUR")
}

// IsValid checks if the money value makes business sense
func (m Money) IsValid() bool {
	return m.Amount >= 0 && m.Currency != "" && len(m.Currency) == 3
}

// Equals compares two Money values for equality
func (m Money) Equals(other Money) bool {
	return m.Amount == other.Amount && m.Currency == other.Currency
}

// String provides a formatted representation of money
func (m Money) String() string {
	return fmt.Sprintf("%.2f %s", m.Amount, m.Currency)
}

// PaymentMethod represents how the payment was made
// This is another value object that encapsulates payment method details
type PaymentMethod struct {
	Type   PaymentMethodType
	Details interface{} // Specific details based on type
}

// PaymentMethodType defines the types of payment methods we support
type PaymentMethodType int

const (
	PaymentMethodCreditCard PaymentMethodType = iota
	PaymentMethodBankTransfer
	PaymentMethodDigitalWallet
)

// String provides human-readable payment method names
func (pmt PaymentMethodType) String() string {
	switch pmt {
	case PaymentMethodCreditCard:
		return "credit_card"
	case PaymentMethodBankTransfer:
		return "bank_transfer"
	case PaymentMethodDigitalWallet:
		return "digital_wallet"
	default:
		return "unknown"
	}
}

// Specific payment method details - these are value objects too

// CreditCardDetails contains credit card payment information
type CreditCardDetails struct {
	MaskedNumber   string // "**** **** **** 1234"
	ExpiryMonth    string // "12"
	ExpiryYear     string // "2025"
	CardholderName string
	Brand          string // "Visa", "MasterCard", etc.
}

// BankTransferDetails contains bank transfer payment information
type BankTransferDetails struct {
	BankName       string
	AccountNumber  string // Masked: "****1234"
	RoutingNumber  string
	AccountHolder  string
}

// DigitalWalletDetails contains digital wallet payment information
type DigitalWalletDetails struct {
	Provider  string // "PayPal", "Apple Pay", etc.
	WalletID  string
	Email     string
}

// Domain Events - these represent important business events that occurred

// PaymentProcessedEvent is raised when a payment is successfully processed
type PaymentProcessedEvent struct {
	PaymentID     string
	OrderID       string
	UserID        string
	Amount        Money
	ProcessedAt   time.Time
	TransactionID string
}

// PaymentFailedEvent is raised when a payment fails
type PaymentFailedEvent struct {
	PaymentID   string
	OrderID     string
	UserID      string
	Amount      Money
	FailedAt    time.Time
	Reason      string
}

// Constructor functions - these ensure our domain objects are created correctly

// NewPayment creates a new payment with business rules validation
func NewPayment(orderID, userID string, amount Money, paymentMethod PaymentMethod, description string) (*Payment, error) {
	// Business rule validation
	if orderID == "" {
		return nil, ErrInvalidOrderID
	}
	if userID == "" {
		return nil, ErrInvalidUserID
	}
	if !amount.IsValid() {
		return nil, ErrInvalidAmount
	}
	if amount.Amount <= 0 {
		return nil, ErrInvalidAmount
	}

	// Generate unique identifiers
	id := uuid.New().String()
	transactionID := generateTransactionID()

	return &Payment{
		id:            id,
		transactionID: transactionID,
		orderID:       orderID,
		userID:        userID,
		amount:        amount,
		paymentMethod: paymentMethod,
		status:        PaymentStatusPending,
		message:       "Payment initiated",
		createdAt:     time.Now(),
		description:   description,
		metadata:      make(map[string]string),
	}, nil
}

// Business methods - these encapsulate business logic and rules

// Process simulates payment processing with business rules
func (p *Payment) Process(processingTimeMs int, successRate float64) error {
	// Business rule: can only process pending payments
	if p.status != PaymentStatusPending {
		return ErrPaymentNotPending
	}

	// Simulate processing delay
	time.Sleep(time.Duration(processingTimeMs) * time.Millisecond)

	// Simulate success/failure based on success rate
	// In a real system, this would integrate with payment processors
	if shouldSucceed(successRate) {
		return p.markAsCompleted()
	}
	
	return p.markAsFailed("Payment processor declined the transaction")
}

// markAsCompleted transitions payment to completed status
func (p *Payment) markAsCompleted() error {
	if p.status != PaymentStatusPending {
		return ErrInvalidStatusTransition
	}
	
	now := time.Now()
	p.status = PaymentStatusCompleted
	p.processedAt = &now
	p.message = "Payment completed successfully"
	
	return nil
}

// markAsFailed transitions payment to failed status
func (p *Payment) markAsFailed(reason string) error {
	if p.status != PaymentStatusPending {
		return ErrInvalidStatusTransition
	}
	
	p.status = PaymentStatusFailed
	p.message = reason
	
	return nil
}

// Cancel cancels a pending payment
func (p *Payment) Cancel(reason string) error {
	if p.status != PaymentStatusPending {
		return ErrCannotCancelNonPendingPayment
	}
	
	p.status = PaymentStatusCancelled
	p.message = reason
	
	return nil
}

// Refund processes a refund for a completed payment
func (p *Payment) Refund(amount Money, reason string) error {
	if p.status != PaymentStatusCompleted && p.status != PaymentStatusPartiallyRefunded {
		return ErrCannotRefundNonCompletedPayment
	}
	
	if !amount.IsValid() || amount.Amount <= 0 {
		return ErrInvalidRefundAmount
	}
	
	if amount.Currency != p.amount.Currency {
		return ErrCurrencyMismatch
	}
	
	// For simplicity, we'll just change status
	// In real systems, you'd track refund amounts
	if amount.Amount >= p.amount.Amount {
		p.status = PaymentStatusRefunded
		p.message = fmt.Sprintf("Fully refunded: %s", reason)
	} else {
		p.status = PaymentStatusPartiallyRefunded
		p.message = fmt.Sprintf("Partially refunded %.2f %s: %s", amount.Amount, amount.Currency, reason)
	}
	
	return nil
}

// Getter methods - these provide controlled access to internal state

func (p *Payment) ID() string { return p.id }
func (p *Payment) TransactionID() string { return p.transactionID }
func (p *Payment) OrderID() string { return p.orderID }
func (p *Payment) UserID() string { return p.userID }
func (p *Payment) Amount() Money { return p.amount }
func (p *Payment) PaymentMethod() PaymentMethod { return p.paymentMethod }
func (p *Payment) Status() PaymentStatus { return p.status }
func (p *Payment) Message() string { return p.message }
func (p *Payment) CreatedAt() time.Time { return p.createdAt }
func (p *Payment) ProcessedAt() *time.Time { return p.processedAt }
func (p *Payment) Description() string { return p.description }

// IsCompleted is a convenience method for checking if payment succeeded
func (p *Payment) IsCompleted() bool {
	return p.status == PaymentStatusCompleted
}

// IsFailed is a convenience method for checking if payment failed
func (p *Payment) IsFailed() bool {
	return p.status == PaymentStatusFailed
}

// Domain Errors - these represent business rule violations

var (
	ErrInvalidOrderID                    = errors.New("order ID cannot be empty")
	ErrInvalidUserID                     = errors.New("user ID cannot be empty")
	ErrInvalidAmount                     = errors.New("amount must be positive and have valid currency")
	ErrPaymentNotPending                 = errors.New("payment is not in pending status")
	ErrInvalidStatusTransition           = errors.New("invalid payment status transition")
	ErrCannotCancelNonPendingPayment     = errors.New("can only cancel pending payments")
	ErrCannotRefundNonCompletedPayment   = errors.New("can only refund completed or partially refunded payments")
	ErrInvalidRefundAmount               = errors.New("refund amount must be positive")
	ErrCurrencyMismatch                  = errors.New("refund currency must match payment currency")
)

// Helper functions

// generateTransactionID creates a unique transaction identifier
// In a real system, this might integrate with external payment processors
func generateTransactionID() string {
	return fmt.Sprintf("txn_%d_%s", time.Now().Unix(), uuid.New().String()[:8])
}

// shouldSucceed simulates payment success based on configured success rate
// This is where you'd integrate with real payment processors
func shouldSucceed(successRate float64) bool {
	// Simple simulation - in practice this would call external payment APIs
	return time.Now().UnixNano()%100 < int64(successRate*100)
}

// Repository interface - this defines how we persist payments
// The actual implementation will be in the repository layer

// PaymentRepository defines the contract for payment persistence
// This follows the Repository pattern from Domain-Driven Design
type PaymentRepository interface {
	// Save persists a payment to storage
	Save(payment *Payment) error
	
	// FindByID retrieves a payment by its unique identifier
	FindByID(id string) (*Payment, error)
	
	// FindByTransactionID retrieves a payment by transaction ID
	FindByTransactionID(transactionID string) (*Payment, error)
	
	// FindByOrderID retrieves payments associated with an order
	FindByOrderID(orderID string) ([]*Payment, error)
	
	// FindByUserID retrieves payments made by a specific user
	FindByUserID(userID string) ([]*Payment, error)
}