// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"fmt"
	"sync"
)

// DNSHandlerRegistry is a registry for DNSHandlerCreatorFunction.
type DNSHandlerRegistry struct {
	lock     sync.RWMutex
	registry map[string]DNSHandlerCreatorFunction
}

var _ DNSHandlerFactory = &DNSHandlerRegistry{}

// NewDNSHandlerRegistry creates a new DNSHandlerRegistry.
func NewDNSHandlerRegistry() *DNSHandlerRegistry {
	return &DNSHandlerRegistry{
		registry: make(map[string]DNSHandlerCreatorFunction),
	}
}

// Register registers a DNSHandler.
func (r *DNSHandlerRegistry) Register(providerType string, creator DNSHandlerCreatorFunction) {
	r.lock.Lock()
	defer r.lock.Unlock()
	if _, ok := r.registry[providerType]; ok {
		panic("handler already registered")
	}
	r.registry[providerType] = creator
}

// Get returns a DNSHandler by provider type.
func (r *DNSHandlerRegistry) Get(providerType string) DNSHandlerCreatorFunction {
	r.lock.RLock()
	defer r.lock.RUnlock()
	return r.registry[providerType]
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
	creator, ok := r.registry[providerType]
	if !ok {
		return nil, fmt.Errorf("provider type %q not found in registry", providerType)
	}
	return creator(config)
}
