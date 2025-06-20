// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"fmt"
	"sync"

	"k8s.io/utils/clock"

	"github.com/gardener/external-dns-management/pkg/dnsman2/apis/config"
)

// DNSHandlerRegistry is a registry for DNSHandlerCreatorFunction.
type DNSHandlerRegistry struct {
	lock     sync.RWMutex
	clock    clock.Clock
	registry map[string]dnsHandlerCreatorConfig
}

type dnsHandlerCreatorConfig struct {
	creator           DNSHandlerCreatorFunction
	defaultRateLimits *config.RateLimiterOptions
	mapper            TargetsMapper
}

var _ DNSHandlerFactory = &DNSHandlerRegistry{}

// AddToRegistryFunc is a function type that can be used to add a DNS handler to the registry.
type AddToRegistryFunc func(registry *DNSHandlerRegistry)

// NewDNSHandlerRegistry creates a new DNSHandlerRegistry.
func NewDNSHandlerRegistry(clock clock.Clock) *DNSHandlerRegistry {
	return &DNSHandlerRegistry{
		clock:    clock,
		registry: make(map[string]dnsHandlerCreatorConfig),
	}
}

// Register registers a DNSHandler.
func (r *DNSHandlerRegistry) Register(
	providerType string,
	creator DNSHandlerCreatorFunction,
	defaultRateLimits *config.RateLimiterOptions,
	mapper TargetsMapper,
) {
	r.lock.Lock()
	defer r.lock.Unlock()
	if _, ok := r.registry[providerType]; ok {
		panic("handler already registered")
	}
	r.registry[providerType] = dnsHandlerCreatorConfig{
		creator:           creator,
		defaultRateLimits: defaultRateLimits,
		mapper:            mapper,
	}
}

// Get returns a DNSHandler by provider type.
func (r *DNSHandlerRegistry) Get(providerType string) DNSHandlerCreatorFunction {
	r.lock.RLock()
	defer r.lock.RUnlock()
	return r.registry[providerType].creator
}

// ListProviderTypes returns a list of all registered provider types.
func (r *DNSHandlerRegistry) ListProviderTypes() []string {
	r.lock.RLock()
	defer r.lock.RUnlock()
	types := make([]string, 0, len(r.registry))
	for providerType := range r.registry {
		types = append(types, providerType)
	}
	return types
}

// Supports checks if the registry supports the given provider type.
func (r *DNSHandlerRegistry) Supports(providerType string) bool {
	r.lock.RLock()
	defer r.lock.RUnlock()
	_, ok := r.registry[providerType]
	return ok
}

// Create creates a DNSHandler for the given provider type and config.
func (r *DNSHandlerRegistry) Create(providerType string, config *DNSHandlerConfig) (DNSHandler, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()
	creatorConfig, ok := r.registry[providerType]
	if !ok {
		return nil, fmt.Errorf("provider type %q not found in registry", providerType)
	}

	if err := config.SetRateLimiter(config.GlobalConfig.ProviderAdvancedOptions[providerType].RateLimits, creatorConfig.defaultRateLimits, r.clock); err != nil {
		return nil, fmt.Errorf("failed to set rate limiter for provider type %q: %w", providerType, err)
	}
	return creatorConfig.creator(config)
}

func (r *DNSHandlerRegistry) GetTargetsMapper(providerType string) (TargetsMapper, error) {
	r.lock.RLock()
	defer r.lock.RUnlock()

	creatorConfig, ok := r.registry[providerType]
	if !ok {
		return nil, fmt.Errorf("provider type %q not found in registry", providerType)
	}
	return creatorConfig.mapper, nil
}
