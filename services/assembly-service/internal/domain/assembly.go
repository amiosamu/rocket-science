package domain

import (
	"time"

	"github.com/google/uuid"
)

// AssemblyStatus represents the current status of an assembly process
type AssemblyStatus int

const (
	AssemblyStatusPending AssemblyStatus = iota
	AssemblyStatusInProgress
	AssemblyStatusCompleted
	AssemblyStatusFailed
)

func (s AssemblyStatus) String() string {
	switch s {
	case AssemblyStatusPending:
		return "pending"
	case AssemblyStatusInProgress:
		return "in_progress"
	case AssemblyStatusCompleted:
		return "completed"
	case AssemblyStatusFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// AssemblyQuality represents the quality of the assembled rocket
type AssemblyQuality int

const (
	AssemblyQualityStandard AssemblyQuality = iota
	AssemblyQualityHigh
	AssemblyQualityPremium
)

func (q AssemblyQuality) String() string {
	switch q {
	case AssemblyQualityStandard:
		return "standard"
	case AssemblyQualityHigh:
		return "high"
	case AssemblyQualityPremium:
		return "premium"
	default:
		return "standard"
	}
}

// RocketComponent represents a component used in rocket assembly
type RocketComponent struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Weight      int32  `json:"weight"`      // in grams
	Dimensions  string `json:"dimensions"`  // e.g., "10x5x3 cm"
	Material    string `json:"material"`    // e.g., "aluminum", "carbon_fiber"
	Criticality string `json:"criticality"` // "low", "medium", "high", "critical"
}

// Assembly represents the rocket assembly process
type Assembly struct {
	ID                       string            `json:"id"`
	OrderID                  string            `json:"order_id"`
	UserID                   string            `json:"user_id"`
	Status                   AssemblyStatus    `json:"status"`
	Components               []RocketComponent `json:"components"`
	Quality                  AssemblyQuality   `json:"quality"`
	EstimatedDurationSeconds int32             `json:"estimated_duration_seconds"`
	ActualDurationSeconds    int32             `json:"actual_duration_seconds"`
	StartedAt                *time.Time        `json:"started_at,omitempty"`
	CompletedAt              *time.Time        `json:"completed_at,omitempty"`
	FailedAt                 *time.Time        `json:"failed_at,omitempty"`
	FailureReason            string            `json:"failure_reason,omitempty"`
	ErrorCode                string            `json:"error_code,omitempty"`
	CreatedAt                time.Time         `json:"created_at"`
	UpdatedAt                time.Time         `json:"updated_at"`
}

// NewAssembly creates a new assembly instance
func NewAssembly(orderID, userID string, components []RocketComponent) *Assembly {
	now := time.Now()
	return &Assembly{
		ID:                       uuid.New().String(),
		OrderID:                  orderID,
		UserID:                   userID,
		Status:                   AssemblyStatusPending,
		Components:               components,
		Quality:                  AssemblyQualityStandard,
		EstimatedDurationSeconds: 10, // Simulated 10 second assembly
		CreatedAt:                now,
		UpdatedAt:                now,
	}
}

// Start begins the assembly process
func (a *Assembly) Start() {
	now := time.Now()
	a.Status = AssemblyStatusInProgress
	a.StartedAt = &now
	a.UpdatedAt = now
}

// Complete marks the assembly as completed
func (a *Assembly) Complete() {
	now := time.Now()
	a.Status = AssemblyStatusCompleted
	a.CompletedAt = &now
	a.UpdatedAt = now

	// Calculate actual duration if started
	if a.StartedAt != nil {
		a.ActualDurationSeconds = int32(now.Sub(*a.StartedAt).Seconds())
	}

	// Determine quality based on components and timing
	a.determineQuality()
}

// Fail marks the assembly as failed
func (a *Assembly) Fail(reason, errorCode string) {
	now := time.Now()
	a.Status = AssemblyStatusFailed
	a.FailedAt = &now
	a.FailureReason = reason
	a.ErrorCode = errorCode
	a.UpdatedAt = now
}

// determineQuality calculates assembly quality based on components and performance
func (a *Assembly) determineQuality() {
	// Simple quality determination logic
	premiumComponents := 0
	highQualityComponents := 0

	for _, component := range a.Components {
		switch component.Material {
		case "carbon_fiber", "titanium":
			premiumComponents++
		case "aluminum", "steel":
			highQualityComponents++
		}
	}

	// Quality based on component materials and assembly timing
	if premiumComponents > len(a.Components)/2 {
		a.Quality = AssemblyQualityPremium
	} else if highQualityComponents > len(a.Components)/3 {
		a.Quality = AssemblyQualityHigh
	} else {
		a.Quality = AssemblyQualityStandard
	}

	// Adjust quality based on assembly timing
	if a.ActualDurationSeconds > a.EstimatedDurationSeconds*2 {
		// Took too long, reduce quality
		if a.Quality == AssemblyQualityPremium {
			a.Quality = AssemblyQualityHigh
		} else if a.Quality == AssemblyQualityHigh {
			a.Quality = AssemblyQualityStandard
		}
	}
}

// IsCompleted returns true if the assembly is completed
func (a *Assembly) IsCompleted() bool {
	return a.Status == AssemblyStatusCompleted
}

// IsFailed returns true if the assembly has failed
func (a *Assembly) IsFailed() bool {
	return a.Status == AssemblyStatusFailed
}

// IsInProgress returns true if the assembly is in progress
func (a *Assembly) IsInProgress() bool {
	return a.Status == AssemblyStatusInProgress
}

// GetDuration returns the duration of the assembly process
func (a *Assembly) GetDuration() time.Duration {
	if a.StartedAt == nil {
		return 0
	}

	endTime := time.Now()
	if a.CompletedAt != nil {
		endTime = *a.CompletedAt
	} else if a.FailedAt != nil {
		endTime = *a.FailedAt
	}

	return endTime.Sub(*a.StartedAt)
}
