// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package istio

import (
	"maps"
	"slices"
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ObjectToGatewaysState is a state that maps Ingress and Service objects to the Istio Gateways that reference them.
type ObjectToGatewaysState struct {
	lock              sync.Mutex
	ingressToGateways map[client.ObjectKey]map[client.ObjectKey]struct{}
	serviceToGateways map[client.ObjectKey]map[client.ObjectKey]struct{}
}

// NewObjectToGatewaysState creates a new ObjectToGatewaysState.
func NewObjectToGatewaysState() *ObjectToGatewaysState {
	return &ObjectToGatewaysState{
		ingressToGateways: map[client.ObjectKey]map[client.ObjectKey]struct{}{},
		serviceToGateways: map[client.ObjectKey]map[client.ObjectKey]struct{}{},
	}
}

// AddIngress adds a mapping from the given Ingress to the given Gateway.
func (s *ObjectToGatewaysState) AddIngress(ingress, gateway client.Object) {
	s.lock.Lock()
	defer s.lock.Unlock()

	add(s.ingressToGateways, client.ObjectKeyFromObject(ingress), client.ObjectKeyFromObject(gateway))
}

// AddService adds a mapping from the given Service to the given Gateway.
func (s *ObjectToGatewaysState) AddService(service, gateway client.Object) {
	s.lock.Lock()
	defer s.lock.Unlock()

	add(s.serviceToGateways, client.ObjectKeyFromObject(service), client.ObjectKeyFromObject(gateway))
}

// RemoveIngress removes the mapping from the given Ingress to all Gateways.
func (s *ObjectToGatewaysState) RemoveIngress(ingress client.Object) {
	s.lock.Lock()
	defer s.lock.Unlock()

	delete(s.ingressToGateways, client.ObjectKeyFromObject(ingress))
}

// RemoveService removes the mapping from the given Service to all Gateways.
func (s *ObjectToGatewaysState) RemoveService(service client.Object) {
	s.lock.Lock()
	defer s.lock.Unlock()

	delete(s.serviceToGateways, client.ObjectKeyFromObject(service))
}

// GetGatewaysForIngress returns the keys of the Gateways that reference the given Ingress.
func (s *ObjectToGatewaysState) GetGatewaysForIngress(ingress client.Object) []client.ObjectKey {
	s.lock.Lock()
	defer s.lock.Unlock()

	return get(s.ingressToGateways, client.ObjectKeyFromObject(ingress))
}

// GetGatewaysForService returns the keys of the Gateways that reference the given Service.
func (s *ObjectToGatewaysState) GetGatewaysForService(service client.Object) []client.ObjectKey {
	s.lock.Lock()
	defer s.lock.Unlock()

	return get(s.serviceToGateways, client.ObjectKeyFromObject(service))
}

// RemoveGateway removes the mapping from the given Gateway to all Ingress and Service objects.
func (s *ObjectToGatewaysState) RemoveGateway(gateway client.Object) {
	s.lock.Lock()
	defer s.lock.Unlock()

	gatewayKey := client.ObjectKeyFromObject(gateway)
	removeValue(s.ingressToGateways, gatewayKey)
	removeValue(s.serviceToGateways, gatewayKey)
}

// RemoveGatewayFromIngressMappings removes the mapping from the given Gateway to all Ingress objects.
func (s *ObjectToGatewaysState) RemoveGatewayFromIngressMappings(gateway client.Object) {
	s.lock.Lock()
	defer s.lock.Unlock()

	removeValue(s.ingressToGateways, client.ObjectKeyFromObject(gateway))
}

// RemoveGatewayFromServiceMappings removes the mapping from the given Gateway to all Service objects.
func (s *ObjectToGatewaysState) RemoveGatewayFromServiceMappings(gateway client.Object) {
	s.lock.Lock()
	defer s.lock.Unlock()

	removeValue(s.serviceToGateways, client.ObjectKeyFromObject(gateway))
}

func add(m map[client.ObjectKey]map[client.ObjectKey]struct{}, key, value client.ObjectKey) {
	if _, exists := m[key]; !exists {
		m[key] = map[client.ObjectKey]struct{}{}
	}
	m[key][value] = struct{}{}
}

func get(m map[client.ObjectKey]map[client.ObjectKey]struct{}, key client.ObjectKey) []client.ObjectKey {
	value, exists := m[key]
	if !exists {
		return nil
	}
	return slices.Collect(maps.Keys(value))
}

func removeValue(m map[client.ObjectKey]map[client.ObjectKey]struct{}, value client.ObjectKey) {
	for _, gatewayKeys := range m {
		delete(gatewayKeys, value)
	}
}
