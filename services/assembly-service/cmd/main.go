package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"

	"github.com/amiosamu/rocket-science/services/assembly-service/internal/container"
)

func main() {
	// Create application context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize dependency container
	fmt.Println("🚀 Starting Assembly Service...")
	container, err := container.NewContainer()
	if err != nil {
		fmt.Printf("❌ Failed to initialize container: %v\n", err)
		os.Exit(1)
	}

	container.Logger.Info(ctx, "Assembly service starting", map[string]interface{}{
		"service_name":    container.Config.Service.Name,
		"service_version": container.Config.Service.Version,
		"environment":     container.Config.Service.Environment,
		"port":            container.Config.Service.Port,
	})

	// Setup graceful shutdown
	var wg sync.WaitGroup
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)

	// Start Kafka consumer
	wg.Add(1)
	go func() {
		defer wg.Done()

		container.Logger.Info(ctx, "Starting Kafka consumer")
		if err := container.AssemblyConsumer.Start(ctx); err != nil {
			container.Logger.Error(ctx, "Kafka consumer failed", err, nil)
			cancel() // Cancel context to signal other goroutines to stop
		}
	}()

	// Start health server
	wg.Add(1)
	go func() {
		defer wg.Done()

		container.Logger.Info(ctx, "Starting health server")
		if err := container.HealthServer.Start(); err != nil {
			container.Logger.Error(ctx, "Health server failed to start", err, nil)
			cancel() // Cancel context to signal other goroutines to stop
		}

		// Wait for context cancellation
		<-ctx.Done()
		container.Logger.Info(ctx, "Health server stopping")
	}()

	// Log service startup completion
	container.Logger.Info(ctx, "🎉 Assembly service started successfully", map[string]interface{}{
		"kafka_brokers":       container.Config.Kafka.Consumer.Brokers,
		"kafka_topics":        container.Config.Kafka.Consumer.Topics,
		"simulation_duration": container.Config.Assembly.SimulationDuration.String(),
		"max_concurrent":      container.Config.Assembly.MaxConcurrentAssemblies,
		"failure_rate":        container.Config.Assembly.FailureRate,
	})

	fmt.Printf("✅ Assembly Service is running!\n")
	fmt.Printf("🏥 Health endpoints: http://localhost:8082/health\n")
	fmt.Printf("📊 Simulation Duration: %s\n", container.Config.Assembly.SimulationDuration)
	fmt.Printf("🔄 Max Concurrent Assemblies: %d\n", container.Config.Assembly.MaxConcurrentAssemblies)
	fmt.Printf("⚠️  Failure Rate: %.1f%%\n", container.Config.Assembly.FailureRate*100)
	fmt.Printf("📡 Kafka Brokers: %v\n", container.Config.Kafka.Consumer.Brokers)
	fmt.Printf("📥 Listening for payment events on: %v\n", container.Config.Kafka.Consumer.Topics)
	fmt.Printf("📤 Publishing assembly events to:\n")
	fmt.Printf("   - Started: %s\n", container.Config.Kafka.Topics.AssemblyStarted)
	fmt.Printf("   - Completed: %s\n", container.Config.Kafka.Topics.AssemblyCompleted)
	fmt.Printf("   - Failed: %s\n", container.Config.Kafka.Topics.AssemblyFailed)
	fmt.Println("\n🛑 Press Ctrl+C to stop the service")

	// Wait for shutdown signal
	<-shutdown

	container.Logger.Info(ctx, "🛑 Shutdown signal received, starting graceful shutdown")
	fmt.Println("\n🛑 Shutdown signal received, starting graceful shutdown...")

	// Cancel context to signal all goroutines to stop
	cancel()

	// Create shutdown context with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), container.Config.Service.GracefulTimeout)
	defer shutdownCancel()

	// Close container dependencies
	go func() {
		if err := container.Close(); err != nil {
			container.Logger.Error(shutdownCtx, "Error during container shutdown", err, nil)
		}
	}()

	// Wait for all goroutines to finish or timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		container.Logger.Info(shutdownCtx, "✅ Graceful shutdown completed")
		fmt.Println("✅ Graceful shutdown completed")
	case <-shutdownCtx.Done():
		container.Logger.Warn(shutdownCtx, "⚠️ Graceful shutdown timed out")
		fmt.Println("⚠️ Graceful shutdown timed out")
	}

	fmt.Println("👋 Assembly Service stopped")
}
