/*
 * Copyright 2023 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 *
 */

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
