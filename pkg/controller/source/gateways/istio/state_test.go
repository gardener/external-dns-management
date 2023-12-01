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
	"github.com/gardener/controller-manager-library/pkg/resources"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gardener/external-dns-management/pkg/controller/source/service"
)

var _ = Describe("resourcesState", func() {
	var (
		state           *resourcesState
		gatewayName1    resources.ObjectName
		gatewayName2    resources.ObjectName
		serviceKey1     resources.ObjectKey
		serviceKey2     resources.ObjectKey
		virtualService1 resources.ObjectName
		virtualService2 resources.ObjectName
		gatewayNames    []resources.ObjectName
	)

	BeforeEach(func() {
		state = newState()
		virtualService1 = resources.NewObjectName("foo", "route1")
		virtualService2 = resources.NewObjectName("foo", "route2")
		gatewayName1 = resources.NewObjectName("foo", "gateway1")
		gatewayName2 = resources.NewObjectName("foo", "gateway2")
		serviceKey1 = resources.NewKey(service.MainResource, "foo", "s1")
		serviceKey2 = resources.NewKey(service.MainResource, "foo", "s2")
		gatewayNames = []resources.ObjectName{gatewayName1, gatewayName2}
	})

	Describe("AddTargetSource", func() {
		Context("when adding a new target source", func() {
			It("should add the gatewayw to the state", func() {
				state.AddTargetSource(serviceKey1, gatewayNames)
				state.AddTargetSource(serviceKey2, gatewayNames)
				Expect(state.TargetSourcesCount()).To(Equal(2))
				Expect(state.MatchingGatewaysByTargetSource(serviceKey1)).To(ContainElements(gatewayName1, gatewayName2))
				By("update", func() {
					state.AddTargetSource(serviceKey1, []resources.ObjectName{gatewayName1})
					Expect(state.MatchingGatewaysByTargetSource(serviceKey1)).To(ContainElements(gatewayName1))
					Expect(state.MatchingGatewaysByTargetSource(serviceKey2)).To(ContainElements(gatewayName1, gatewayName2))
					state.AddTargetSource(serviceKey1, []resources.ObjectName{})
					Expect(state.MatchingGatewaysByTargetSource(serviceKey1)).To(BeEmpty())
				})
			})
		})
	})

	Describe("RemoveTargetSource", func() {
		Context("when removing an existing target source", func() {
			BeforeEach(func() {
				state.AddTargetSource(serviceKey1, gatewayNames)
				state.AddTargetSource(serviceKey2, gatewayNames)
			})

			It("should remove the gateways from the state", func() {
				state.RemoveTargetSource(serviceKey1)
				Expect(state.MatchingGatewaysByTargetSource(serviceKey1)).To(BeEmpty())
				Expect(state.MatchingGatewaysByTargetSource(serviceKey2)).To(ContainElements(gatewayName1, gatewayName2))
				Expect(state.TargetSourcesCount()).To(Equal(1))
			})
		})
	})

	Describe("AddVirtualService", func() {
		Context("when adding a new virtual service", func() {
			It("should add the gateways of the route to the state", func() {
				state.AddVirtualService(virtualService1, resources.NewObjectNameSetByArray(gatewayNames))
				state.AddVirtualService(virtualService2, resources.NewObjectNameSetByArray(gatewayNames))
				Expect(state.MatchingGatewaysByVirtualService(virtualService1)).To(ContainElements(gatewayName1, gatewayName2))
				By("update", func() {
					state.AddVirtualService(virtualService1, resources.NewObjectNameSet(gatewayName2))
					Expect(state.MatchingGatewaysByVirtualService(virtualService1)).To(ContainElements(gatewayName2))
					Expect(state.MatchingGatewaysByVirtualService(virtualService2)).To(ContainElements(gatewayName1, gatewayName2))
					state.AddVirtualService(virtualService1, resources.NewObjectNameSet())
					Expect(state.MatchingGatewaysByVirtualService(virtualService1)).To(BeEmpty())
					state.AddVirtualService(virtualService1, resources.NewObjectNameSet(gatewayName1))
					Expect(state.MatchingGatewaysByVirtualService(virtualService1)).To(ContainElements(gatewayName1))
					Expect(state.MatchingGatewaysByVirtualService(virtualService2)).To(ContainElements(gatewayName1, gatewayName2))
				})
			})
		})
	})

	Describe("RemoveVirtualService", func() {
		Context("when removing an existing route", func() {
			BeforeEach(func() {
				state.AddVirtualService(virtualService1, resources.NewObjectNameSetByArray(gatewayNames))
				state.AddVirtualService(virtualService2, resources.NewObjectNameSetByArray(gatewayNames))
			})

			It("should remove the gateways from the state", func() {
				state.RemoveVirtualService(virtualService1)
				state.RemoveVirtualService(virtualService1)
				Expect(state.MatchingGatewaysByVirtualService(virtualService1)).To(BeEmpty())
				state.RemoveVirtualService(virtualService2)
				Expect(state.MatchingGatewaysByVirtualService(virtualService2)).To(BeEmpty())
				Expect(state.VirtualServicesCount()).To(Equal(0))
			})
		})
	})
})
