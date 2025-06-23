package service

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/amiosamu/rocket-science/services/assembly-service/internal/config"
	"github.com/amiosamu/rocket-science/services/assembly-service/internal/domain"
	"github.com/amiosamu/rocket-science/shared/platform/observability/logging"
	"github.com/amiosamu/rocket-science/shared/platform/observability/metrics"
	"github.com/amiosamu/rocket-science/shared/proto/events"
)

// AssemblyProducer defines the interface for publishing assembly events
type AssemblyProducer interface {
	PublishAssemblyStarted(ctx context.Context, assembly *domain.Assembly) error
	PublishAssemblyCompleted(ctx context.Context, assembly *domain.Assembly) error
	PublishAssemblyFailed(ctx context.Context, assembly *domain.Assembly) error
}

// AssemblyService handles the core assembly business logic
type AssemblyService struct {
	config   config.AssemblyConfig
	producer AssemblyProducer
	logger   logging.Logger
	metrics  metrics.Metrics

	// In-memory storage for active assemblies (in production, this would be in a database)
	activeAssemblies map[string]*domain.Assembly
	mu               sync.RWMutex

	// Channel for managing concurrent assemblies
	assemblySemaphore chan struct{}
}

// NewAssemblyService creates a new assembly service
func NewAssemblyService(
	config config.AssemblyConfig,
	producer AssemblyProducer,
	logger logging.Logger,
	metrics metrics.Metrics,
) *AssemblyService {
	return &AssemblyService{
		config:            config,
		producer:          producer,
		logger:            logger,
		metrics:           metrics,
		activeAssemblies:  make(map[string]*domain.Assembly),
		assemblySemaphore: make(chan struct{}, config.MaxConcurrentAssemblies),
	}
}

// HandlePaymentProcessed processes payment completion and starts rocket assembly
func (s *AssemblyService) HandlePaymentProcessed(ctx context.Context, paymentEvent *events.PaymentProcessedEvent) error {
	s.logger.Info(ctx, "Starting assembly for paid order", map[string]interface{}{
		"order_id":   paymentEvent.OrderId,
		"user_id":    paymentEvent.UserId,
		"payment_id": paymentEvent.PaymentId,
		"amount":     paymentEvent.Amount.Amount,
		"currency":   paymentEvent.Amount.Currency,
	})

	// Record metrics
	s.metrics.IncrementCounter("assembly_requests_total", map[string]string{
		"user_id": paymentEvent.UserId,
	})

	// Extract rocket components from payment metadata (in a real system, this might come from the order service)
	components := s.generateRocketComponents(paymentEvent.OrderId)

	// Create new assembly
	assembly := domain.NewAssembly(paymentEvent.OrderId, paymentEvent.UserId, components)

	// Store assembly in memory
	s.mu.Lock()
	s.activeAssemblies[assembly.ID] = assembly
	s.mu.Unlock()

	// Start assembly process asynchronously
	go s.processAssembly(ctx, assembly)

	s.metrics.IncrementCounter("assemblies_started_total", map[string]string{
		"user_id": paymentEvent.UserId,
	})

	return nil
}

// processAssembly handles the actual assembly process
func (s *AssemblyService) processAssembly(ctx context.Context, assembly *domain.Assembly) {
	// Acquire semaphore to limit concurrent assemblies
	s.assemblySemaphore <- struct{}{}
	defer func() { <-s.assemblySemaphore }()

	s.logger.Info(ctx, "Beginning rocket assembly process", map[string]interface{}{
		"assembly_id": assembly.ID,
		"order_id":    assembly.OrderID,
		"user_id":     assembly.UserID,
		"components":  len(assembly.Components),
	})

	// Start the assembly
	assembly.Start()

	// Update assembly in storage
	s.mu.Lock()
	s.activeAssemblies[assembly.ID] = assembly
	s.mu.Unlock()

	// Publish assembly started event
	if err := s.producer.PublishAssemblyStarted(ctx, assembly); err != nil {
		s.logger.Error(ctx, "Failed to publish assembly started event", err, map[string]interface{}{
			"assembly_id": assembly.ID,
			"order_id":    assembly.OrderID,
		})
	}

	// Simulate assembly process with configurable duration
	s.simulateAssemblyWork(ctx, assembly)

	// Check if assembly should fail (simulate random failures)
	if s.shouldSimulateFailure() {
		s.handleAssemblyFailure(ctx, assembly)
		return
	}

	// Complete the assembly
	assembly.Complete()

	// Update assembly in storage
	s.mu.Lock()
	s.activeAssemblies[assembly.ID] = assembly
	s.mu.Unlock()

	// Publish assembly completed event
	if err := s.producer.PublishAssemblyCompleted(ctx, assembly); err != nil {
		s.logger.Error(ctx, "Failed to publish assembly completed event", err, map[string]interface{}{
			"assembly_id": assembly.ID,
			"order_id":    assembly.OrderID,
		})
	}

	s.logger.Info(ctx, "Rocket assembly completed successfully", map[string]interface{}{
		"assembly_id":       assembly.ID,
		"order_id":          assembly.OrderID,
		"user_id":           assembly.UserID,
		"quality":           assembly.Quality.String(),
		"duration_seconds":  assembly.ActualDurationSeconds,
		"estimated_seconds": assembly.EstimatedDurationSeconds,
	})

	s.metrics.IncrementCounter("assemblies_completed_total", map[string]string{
		"user_id": assembly.UserID,
		"quality": assembly.Quality.String(),
	})

	s.metrics.RecordValue("assembly_duration_seconds", float64(assembly.ActualDurationSeconds), map[string]string{
		"user_id": assembly.UserID,
		"quality": assembly.Quality.String(),
	})
}

// simulateAssemblyWork simulates the rocket assembly process
func (s *AssemblyService) simulateAssemblyWork(ctx context.Context, assembly *domain.Assembly) {
	duration := s.config.SimulationDuration

	s.logger.Debug(ctx, "Simulating assembly work", map[string]interface{}{
		"assembly_id":      assembly.ID,
		"duration_seconds": duration.Seconds(),
		"components":       len(assembly.Components),
	})

	// Add some variability to the assembly time (±20%)
	variability := time.Duration(float64(duration) * 0.2 * (rand.Float64() - 0.5) * 2)
	actualDuration := duration + variability

	// Simulate work by sleeping
	select {
	case <-time.After(actualDuration):
		// Assembly completed normally
		s.logger.Debug(ctx, "Assembly simulation completed", map[string]interface{}{
			"assembly_id":        assembly.ID,
			"actual_duration":    actualDuration.Seconds(),
			"estimated_duration": duration.Seconds(),
		})
	case <-ctx.Done():
		// Context was cancelled
		s.logger.Warn(ctx, "Assembly cancelled due to context cancellation", map[string]interface{}{
			"assembly_id": assembly.ID,
		})
		return
	}
}

// handleAssemblyFailure handles assembly failures
func (s *AssemblyService) handleAssemblyFailure(ctx context.Context, assembly *domain.Assembly) {
	// Determine failure reason
	failureReasons := []string{
		"component_malfunction",
		"quality_check_failed",
		"insufficient_materials",
		"calibration_error",
		"safety_protocol_violation",
	}

	failureCodes := []string{
		"ASM_001",
		"ASM_002",
		"ASM_003",
		"ASM_004",
		"ASM_005",
	}

	index := rand.Intn(len(failureReasons))
	reason := failureReasons[index]
	code := failureCodes[index]

	assembly.Fail(reason, code)

	// Update assembly in storage
	s.mu.Lock()
	s.activeAssemblies[assembly.ID] = assembly
	s.mu.Unlock()

	// Publish assembly failed event
	if err := s.producer.PublishAssemblyFailed(ctx, assembly); err != nil {
		s.logger.Error(ctx, "Failed to publish assembly failed event", err, map[string]interface{}{
			"assembly_id": assembly.ID,
			"order_id":    assembly.OrderID,
		})
	}

	s.logger.Warn(ctx, "Rocket assembly failed", map[string]interface{}{
		"assembly_id":    assembly.ID,
		"order_id":       assembly.OrderID,
		"user_id":        assembly.UserID,
		"failure_reason": reason,
		"error_code":     code,
	})

	s.metrics.IncrementCounter("assemblies_failed_total", map[string]string{
		"user_id":        assembly.UserID,
		"failure_reason": reason,
		"error_code":     code,
	})
}

// shouldSimulateFailure determines if an assembly should fail based on configured failure rate
func (s *AssemblyService) shouldSimulateFailure() bool {
	return rand.Float64() < s.config.FailureRate
}

// generateRocketComponents generates realistic rocket components for an order
func (s *AssemblyService) generateRocketComponents(orderID string) []domain.RocketComponent {
	// In a real system, this would fetch components from the order service or inventory
	// For simulation, we generate a set of standard rocket components

	components := []domain.RocketComponent{
		{
			ID:          fmt.Sprintf("engine-%s", orderID),
			Name:        "Rocket Engine",
			Type:        "engine",
			Weight:      15000, // 15 kg
			Dimensions:  "30x30x60 cm",
			Material:    "aluminum",
			Criticality: "critical",
		},
		{
			ID:          fmt.Sprintf("fuel-tank-%s", orderID),
			Name:        "Fuel Tank",
			Type:        "tank",
			Weight:      8000, // 8 kg
			Dimensions:  "25x25x80 cm",
			Material:    "carbon_fiber",
			Criticality: "critical",
		},
		{
			ID:          fmt.Sprintf("guidance-%s", orderID),
			Name:        "Guidance System",
			Type:        "electronics",
			Weight:      2000, // 2 kg
			Dimensions:  "20x15x10 cm",
			Material:    "aluminum",
			Criticality: "high",
		},
		{
			ID:          fmt.Sprintf("fins-%s", orderID),
			Name:        "Stabilizing Fins",
			Type:        "structure",
			Weight:      1500, // 1.5 kg
			Dimensions:  "40x20x2 cm",
			Material:    "carbon_fiber",
			Criticality: "medium",
		},
		{
			ID:          fmt.Sprintf("parachute-%s", orderID),
			Name:        "Recovery Parachute",
			Type:        "recovery",
			Weight:      800, // 0.8 kg
			Dimensions:  "50x50x15 cm (packed)",
			Material:    "nylon",
			Criticality: "high",
		},
	}

	// Add some randomness to component materials for quality calculation
	materials := []string{"aluminum", "carbon_fiber", "titanium", "steel"}
	for i := range components {
		if rand.Float64() < 0.3 { // 30% chance to upgrade material
			components[i].Material = materials[rand.Intn(len(materials))]
		}
	}

	return components
}

// GetAssembly retrieves an assembly by ID
func (s *AssemblyService) GetAssembly(ctx context.Context, assemblyID string) (*domain.Assembly, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	assembly, exists := s.activeAssemblies[assemblyID]
	if !exists {
		return nil, fmt.Errorf("assembly not found: %s", assemblyID)
	}

	return assembly, nil
}

// GetAssemblyByOrderID retrieves an assembly by order ID
func (s *AssemblyService) GetAssemblyByOrderID(ctx context.Context, orderID string) (*domain.Assembly, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for _, assembly := range s.activeAssemblies {
		if assembly.OrderID == orderID {
			return assembly, nil
		}
	}

	return nil, fmt.Errorf("assembly not found for order: %s", orderID)
}

// ListActiveAssemblies returns all active assemblies
func (s *AssemblyService) ListActiveAssemblies(ctx context.Context) []*domain.Assembly {
	s.mu.RLock()
	defer s.mu.RUnlock()

	assemblies := make([]*domain.Assembly, 0, len(s.activeAssemblies))
	for _, assembly := range s.activeAssemblies {
		assemblies = append(assemblies, assembly)
	}

	return assemblies
}

// GetStats returns service statistics
func (s *AssemblyService) GetStats(ctx context.Context) map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := map[string]interface{}{
		"active_assemblies":      len(s.activeAssemblies),
		"max_concurrent":         s.config.MaxConcurrentAssemblies,
		"current_semaphore_load": len(s.assemblySemaphore),
		"simulation_duration":    s.config.SimulationDuration.String(),
		"failure_rate":           s.config.FailureRate,
	}

	// Count assemblies by status
	statusCounts := make(map[string]int)
	for _, assembly := range s.activeAssemblies {
		statusCounts[assembly.Status.String()]++
	}
	stats["status_counts"] = statusCounts

	return stats
}
