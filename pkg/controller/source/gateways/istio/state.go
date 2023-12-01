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

package istio

import (
	"sync"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/ctxutil"
	"github.com/gardener/controller-manager-library/pkg/resources"
)

var stateKey = ctxutil.SimpleKey("istio-gateways-state")

type resourcesState struct {
	lock sync.Mutex

	sourcesToGateways         map[resources.ObjectKey][]resources.ObjectName
	virtualservicesToGateways map[resources.ObjectName][]resources.ObjectName
}

func newState() *resourcesState {
	return &resourcesState{
		sourcesToGateways:         map[resources.ObjectKey][]resources.ObjectName{},
		virtualservicesToGateways: map[resources.ObjectName][]resources.ObjectName{},
	}
}

func getOrCreateSharedState(c controller.Interface) (*resourcesState, error) {
	state := c.GetEnvironment().GetOrCreateSharedValue(stateKey, func() interface{} {
		return newState()
	}).(*resourcesState)

	return state, nil
}

func (s *resourcesState) AddTargetSource(source resources.ObjectKey, gateways []resources.ObjectName) {
	s.lock.Lock()
	defer s.lock.Unlock()

	if len(gateways) == 0 {
		delete(s.sourcesToGateways, source)
		return
	}

	value := make([]resources.ObjectName, len(gateways))
	copy(value, gateways)

	s.sourcesToGateways[source] = value
}

func (s *resourcesState) RemoveTargetSource(source resources.ObjectKey) {
	s.lock.Lock()
	defer s.lock.Unlock()
	delete(s.sourcesToGateways, source)
}

func (s *resourcesState) MatchingGatewaysByTargetSource(source resources.ObjectKey) []resources.ObjectName {
	s.lock.Lock()
	defer s.lock.Unlock()

	if array := s.sourcesToGateways[source]; array != nil {
		value := make([]resources.ObjectName, len(array))
		copy(value, array)
		return value
	}
	return nil
}

func (s *resourcesState) AddVirtualService(virtualservice resources.ObjectName, gateways resources.ObjectNameSet) []resources.ObjectName {
	s.lock.Lock()
	defer s.lock.Unlock()

	oldGateways := s.virtualservicesToGateways[virtualservice]
	if len(gateways) == 0 {
		delete(s.virtualservicesToGateways, virtualservice)
		return oldGateways
	}

	s.virtualservicesToGateways[virtualservice] = gateways.AsArray()
	return oldGateways
}

func (s *resourcesState) RemoveVirtualService(virtualservice resources.ObjectName) {
	s.lock.Lock()
	defer s.lock.Unlock()
	delete(s.virtualservicesToGateways, virtualservice)
}

func (s *resourcesState) MatchingGatewaysByVirtualService(virtualservice resources.ObjectName) []resources.ObjectName {
	s.lock.Lock()
	defer s.lock.Unlock()

	if array := s.virtualservicesToGateways[virtualservice]; array != nil {
		value := make([]resources.ObjectName, len(array))
		copy(value, array)
		return value
	}
	return nil
}

func (s *resourcesState) VirtualServicesCount() int {
	s.lock.Lock()
	defer s.lock.Unlock()
	return len(s.virtualservicesToGateways)
}

func (s *resourcesState) TargetSourcesCount() int {
	s.lock.Lock()
	defer s.lock.Unlock()
	return len(s.sourcesToGateways)
}
