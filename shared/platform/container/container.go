package container

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"github.com/amiosamu/rocket-science/shared/platform/errors"
)

// Container represents a dependency injection container
type Container struct {
	services map[string]*ServiceDescriptor
	mu       sync.RWMutex
}

// ServiceDescriptor describes how to create and manage a service
type ServiceDescriptor struct {
	Name         string
	Type         reflect.Type
	Lifetime     Lifetime
	Factory      interface{}
	Instance     interface{}
	Dependencies []string
	mu           sync.RWMutex
}

// Lifetime represents the lifetime of a service
type Lifetime int

const (
	// Transient creates a new instance every time
	Transient Lifetime = iota
	// Singleton creates one instance and reuses it
	Singleton
	// Scoped creates one instance per scope (request, etc.)
	Scoped
)

// NewContainer creates a new dependency injection container
func NewContainer() *Container {
	return &Container{
		services: make(map[string]*ServiceDescriptor),
	}
}

// RegisterSingleton registers a service as singleton
func (c *Container) RegisterSingleton(name string, factory interface{}) error {
	return c.register(name, factory, Singleton)
}

// RegisterTransient registers a service as transient
func (c *Container) RegisterTransient(name string, factory interface{}) error {
	return c.register(name, factory, Transient)
}

// RegisterScoped registers a service as scoped
func (c *Container) RegisterScoped(name string, factory interface{}) error {
	return c.register(name, factory, Scoped)
}

// RegisterInstance registers a pre-created instance
func (c *Container) RegisterInstance(name string, instance interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if instance == nil {
		return errors.NewValidation("instance cannot be nil")
	}

	serviceType := reflect.TypeOf(instance)
	if serviceType.Kind() == reflect.Ptr {
		serviceType = serviceType.Elem()
	}

	c.services[name] = &ServiceDescriptor{
		Name:     name,
		Type:     serviceType,
		Lifetime: Singleton,
		Instance: instance,
	}

	return nil
}

// register registers a service with the container
func (c *Container) register(name string, factory interface{}, lifetime Lifetime) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if factory == nil {
		return errors.NewValidation("factory cannot be nil")
	}

	factoryType := reflect.TypeOf(factory)
	if factoryType.Kind() != reflect.Func {
		return errors.NewValidation("factory must be a function")
	}

	if factoryType.NumOut() == 0 {
		return errors.NewValidation("factory must return at least one value")
	}

	serviceType := factoryType.Out(0)
	if serviceType.Kind() == reflect.Ptr {
		serviceType = serviceType.Elem()
	}

	dependencies := make([]string, factoryType.NumIn())
	for i := 0; i < factoryType.NumIn(); i++ {
		paramType := factoryType.In(i)
		if paramType == reflect.TypeOf((*context.Context)(nil)).Elem() {
			dependencies[i] = "context"
		} else {
			dependencies[i] = paramType.String()
		}
	}

	c.services[name] = &ServiceDescriptor{
		Name:         name,
		Type:         serviceType,
		Lifetime:     lifetime,
		Factory:      factory,
		Dependencies: dependencies,
	}

	return nil
}

// Resolve resolves a service by name
func (c *Container) Resolve(ctx context.Context, name string) (interface{}, error) {
	return c.resolve(ctx, name, make(map[string]bool))
}

// ResolveByType resolves a service by type
func (c *Container) ResolveByType(ctx context.Context, serviceType reflect.Type) (interface{}, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for name, descriptor := range c.services {
		if descriptor.Type == serviceType || reflect.PtrTo(descriptor.Type) == serviceType {
			return c.resolve(ctx, name, make(map[string]bool))
		}
	}

	return nil, errors.NewNotFound(fmt.Sprintf("service of type %s not found", serviceType.String()))
}

// resolve resolves a service with circular dependency detection
func (c *Container) resolve(ctx context.Context, name string, resolving map[string]bool) (interface{}, error) {
	if resolving[name] {
		return nil, errors.NewValidation(fmt.Sprintf("circular dependency detected for service %s", name))
	}

	c.mu.RLock()
	descriptor, exists := c.services[name]
	c.mu.RUnlock()

	if !exists {
		return nil, errors.NewNotFound(fmt.Sprintf("service %s not found", name))
	}

	// Return existing instance for singleton
	if descriptor.Lifetime == Singleton {
		descriptor.mu.RLock()
		if descriptor.Instance != nil {
			instance := descriptor.Instance
			descriptor.mu.RUnlock()
			return instance, nil
		}
		descriptor.mu.RUnlock()
	}

	// Create new instance
	resolving[name] = true
	defer delete(resolving, name)

	instance, err := c.createInstance(ctx, descriptor, resolving)
	if err != nil {
		return nil, err
	}

	// Store instance for singleton
	if descriptor.Lifetime == Singleton {
		descriptor.mu.Lock()
		if descriptor.Instance == nil {
			descriptor.Instance = instance
		}
		descriptor.mu.Unlock()
	}

	return instance, nil
}

// createInstance creates a new instance of a service
func (c *Container) createInstance(ctx context.Context, descriptor *ServiceDescriptor, resolving map[string]bool) (interface{}, error) {
	if descriptor.Factory == nil {
		return descriptor.Instance, nil
	}

	factoryValue := reflect.ValueOf(descriptor.Factory)
	factoryType := factoryValue.Type()

	// Prepare arguments for factory function
	args := make([]reflect.Value, factoryType.NumIn())
	for i := 0; i < factoryType.NumIn(); i++ {
		paramType := factoryType.In(i)

		if paramType == reflect.TypeOf((*context.Context)(nil)).Elem() {
			args[i] = reflect.ValueOf(ctx)
		} else {
			// Resolve dependency
			dependency, err := c.resolveDependency(ctx, paramType, resolving)
			if err != nil {
				return nil, errors.Wrap(err, fmt.Sprintf("failed to resolve dependency %s for service %s", paramType.String(), descriptor.Name))
			}
			args[i] = reflect.ValueOf(dependency)
		}
	}

	// Call factory function
	results := factoryValue.Call(args)

	// Check for error in return values
	if len(results) > 1 {
		if errInterface := results[len(results)-1].Interface(); errInterface != nil {
			if err, ok := errInterface.(error); ok {
				return nil, err
			}
		}
	}

	return results[0].Interface(), nil
}

// resolveDependency resolves a dependency by type
func (c *Container) resolveDependency(ctx context.Context, paramType reflect.Type, resolving map[string]bool) (interface{}, error) {
	// Try to find service by exact type match
	c.mu.RLock()
	for name, descriptor := range c.services {
		if descriptor.Type == paramType || reflect.PtrTo(descriptor.Type) == paramType {
			c.mu.RUnlock()
			return c.resolve(ctx, name, resolving)
		}
	}
	c.mu.RUnlock()

	// Try to find service by interface implementation
	if paramType.Kind() == reflect.Interface {
		c.mu.RLock()
		for name, descriptor := range c.services {
			if descriptor.Type.Implements(paramType) || reflect.PtrTo(descriptor.Type).Implements(paramType) {
				c.mu.RUnlock()
				return c.resolve(ctx, name, resolving)
			}
		}
		c.mu.RUnlock()
	}

	return nil, errors.NewNotFound(fmt.Sprintf("no service found for type %s", paramType.String()))
}

// GetService gets a service descriptor by name
func (c *Container) GetService(name string) (*ServiceDescriptor, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	descriptor, exists := c.services[name]
	return descriptor, exists
}

// ListServices returns all registered service names
func (c *Container) ListServices() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	names := make([]string, 0, len(c.services))
	for name := range c.services {
		names = append(names, name)
	}
	return names
}

// Remove removes a service from the container
func (c *Container) Remove(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	delete(c.services, name)
}

// Clear removes all services from the container
func (c *Container) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	c.services = make(map[string]*ServiceDescriptor)
}

// Scope represents a service scope
type Scope struct {
	container *Container
	instances map[string]interface{}
	mu        sync.RWMutex
}

// NewScope creates a new service scope
func (c *Container) NewScope() *Scope {
	return &Scope{
		container: c,
		instances: make(map[string]interface{}),
	}
}

// Resolve resolves a service within this scope
func (s *Scope) Resolve(ctx context.Context, name string) (interface{}, error) {
	s.mu.RLock()
	descriptor, exists := s.container.services[name]
	s.mu.RUnlock()

	if !exists {
		return nil, errors.NewNotFound(fmt.Sprintf("service %s not found", name))
	}

	// For scoped services, check if instance exists in scope
	if descriptor.Lifetime == Scoped {
		s.mu.RLock()
		if instance, exists := s.instances[name]; exists {
			s.mu.RUnlock()
			return instance, nil
		}
		s.mu.RUnlock()
	}

	// Resolve from container
	instance, err := s.container.Resolve(ctx, name)
	if err != nil {
		return nil, err
	}

	// Store in scope for scoped services
	if descriptor.Lifetime == Scoped {
		s.mu.Lock()
		s.instances[name] = instance
		s.mu.Unlock()
	}

	return instance, nil
}

// Dispose disposes of the scope and any disposable instances
func (s *Scope) Dispose() {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, instance := range s.instances {
		if disposable, ok := instance.(Disposable); ok {
			disposable.Dispose()
		}
	}

	s.instances = make(map[string]interface{})
}

// Disposable represents a disposable resource
type Disposable interface {
	Dispose()
}

// ServiceBuilder provides a fluent interface for service registration
type ServiceBuilder struct {
	container *Container
	name      string
	factory   interface{}
	lifetime  Lifetime
}

// NewServiceBuilder creates a new service builder
func (c *Container) NewServiceBuilder(name string) *ServiceBuilder {
	return &ServiceBuilder{
		container: c,
		name:      name,
		lifetime:  Transient,
	}
}

// WithFactory sets the factory function
func (sb *ServiceBuilder) WithFactory(factory interface{}) *ServiceBuilder {
	sb.factory = factory
	return sb
}

// AsSingleton sets the lifetime to singleton
func (sb *ServiceBuilder) AsSingleton() *ServiceBuilder {
	sb.lifetime = Singleton
	return sb
}

// AsTransient sets the lifetime to transient
func (sb *ServiceBuilder) AsTransient() *ServiceBuilder {
	sb.lifetime = Transient
	return sb
}

// AsScoped sets the lifetime to scoped
func (sb *ServiceBuilder) AsScoped() *ServiceBuilder {
	sb.lifetime = Scoped
	return sb
}

// Build registers the service with the container
func (sb *ServiceBuilder) Build() error {
	return sb.container.register(sb.name, sb.factory, sb.lifetime)
}

// HealthChecker provides health check functionality for services
type HealthChecker struct {
	container *Container
}

// NewHealthChecker creates a new health checker
func (c *Container) NewHealthChecker() *HealthChecker {
	return &HealthChecker{container: c}
}

// CheckHealth checks the health of all registered services
func (hc *HealthChecker) CheckHealth(ctx context.Context) map[string]error {
	results := make(map[string]error)
	
	hc.container.mu.RLock()
	services := make(map[string]*ServiceDescriptor)
	for name, descriptor := range hc.container.services {
		services[name] = descriptor
	}
	hc.container.mu.RUnlock()

	for name := range services {
		_, err := hc.container.Resolve(ctx, name)
		results[name] = err
	}

	return results
}

// Validator provides validation functionality for container configuration
type Validator struct {
	container *Container
}

// NewValidator creates a new validator
func (c *Container) NewValidator() *Validator {
	return &Validator{container: c}
}

// ValidateConfiguration validates the container configuration
func (v *Validator) ValidateConfiguration() []error {
	var errors []error

	v.container.mu.RLock()
	defer v.container.mu.RUnlock()

	// Check for circular dependencies
	for name := range v.container.services {
		if err := v.checkCircularDependencies(name, make(map[string]bool)); err != nil {
			errors = append(errors, err)
		}
	}

	// Check for missing dependencies
	for name, descriptor := range v.container.services {
		if descriptor.Factory != nil {
			factoryType := reflect.TypeOf(descriptor.Factory)
			for i := 0; i < factoryType.NumIn(); i++ {
				paramType := factoryType.In(i)
				if paramType != reflect.TypeOf((*context.Context)(nil)).Elem() {
					if !v.canResolveType(paramType) {
						errors = append(errors, fmt.Errorf("service %s has unresolvable dependency of type %s", name, paramType.String()))
					}
				}
			}
		}
	}

	return errors
}

// checkCircularDependencies checks for circular dependencies
func (v *Validator) checkCircularDependencies(name string, visited map[string]bool) error {
	if visited[name] {
		return fmt.Errorf("circular dependency detected for service %s", name)
	}

	descriptor, exists := v.container.services[name]
	if !exists {
		return nil
	}

	visited[name] = true
	defer delete(visited, name)

	if descriptor.Factory != nil {
		factoryType := reflect.TypeOf(descriptor.Factory)
		for i := 0; i < factoryType.NumIn(); i++ {
			paramType := factoryType.In(i)
			if paramType != reflect.TypeOf((*context.Context)(nil)).Elem() {
				for depName, depDescriptor := range v.container.services {
					if depDescriptor.Type == paramType || reflect.PtrTo(depDescriptor.Type) == paramType {
						if err := v.checkCircularDependencies(depName, visited); err != nil {
							return err
						}
					}
				}
			}
		}
	}

	return nil
}

// canResolveType checks if a type can be resolved
func (v *Validator) canResolveType(paramType reflect.Type) bool {
	for _, descriptor := range v.container.services {
		if descriptor.Type == paramType || reflect.PtrTo(descriptor.Type) == paramType {
			return true
		}
		if paramType.Kind() == reflect.Interface && (descriptor.Type.Implements(paramType) || reflect.PtrTo(descriptor.Type).Implements(paramType)) {
			return true
		}
	}
	return false
}