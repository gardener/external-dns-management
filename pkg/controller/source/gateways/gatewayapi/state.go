// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package gatewayapi

import (
	"sync"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/ctxutil"
	"github.com/gardener/controller-manager-library/pkg/resources"
)

var stateKey = ctxutil.SimpleKey("gatewayapi-gateways-state")

type routesState struct {
	lock sync.Mutex

	routesToGateways map[resources.ObjectName][]resources.ObjectName
}

func newState() *routesState {
	return &routesState{
		routesToGateways: map[resources.ObjectName][]resources.ObjectName{},
	}
}

func getOrCreateSharedState(c controller.Interface) (*routesState, error) {
	state := c.GetEnvironment().GetOrCreateSharedValue(stateKey, func() interface{} {
		return newState()
	}).(*routesState)

	return state, nil
}

func (s *routesState) AddRoute(route resources.ObjectName, gateways resources.ObjectNameSet) []resources.ObjectName {
	s.lock.Lock()
	defer s.lock.Unlock()

	oldGateways := s.routesToGateways[route]
	if len(gateways) == 0 {
		delete(s.routesToGateways, route)
		return oldGateways
	}

	s.routesToGateways[route] = gateways.AsArray()
	return oldGateways
}

func (s *routesState) RemoveRoute(route resources.ObjectName) {
	s.lock.Lock()
	defer s.lock.Unlock()

	delete(s.routesToGateways, route)
}

func (s *routesState) MatchingGatewaysByRoute(route resources.ObjectName) []resources.ObjectName {
	s.lock.Lock()
	defer s.lock.Unlock()

	if array := s.routesToGateways[route]; array != nil {
		value := make([]resources.ObjectName, len(array))
		copy(value, array)
		return value
	}
	return nil
}

func (s *routesState) RoutesCount() int {
	s.lock.Lock()
	defer s.lock.Unlock()
	return len(s.routesToGateways)
}
