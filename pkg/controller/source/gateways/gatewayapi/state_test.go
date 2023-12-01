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
	"github.com/gardener/controller-manager-library/pkg/resources"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("routesState", func() {
	var (
		state        *routesState
		gatewayName1 resources.ObjectName
		gatewayName2 resources.ObjectName
		routeName1   resources.ObjectName
		routeName2   resources.ObjectName
		gatewayNames resources.ObjectNameSet
	)

	BeforeEach(func() {
		state = newState()
		gatewayName1 = resources.NewObjectName("foo", "gateway1")
		gatewayName2 = resources.NewObjectName("foo", "gateway2")
		routeName1 = resources.NewObjectName("foo", "route1")
		routeName2 = resources.NewObjectName("foo", "route2")
		gatewayNames = resources.NewObjectNameSet(gatewayName1, gatewayName2)
	})

	Describe("AddRoute", func() {
		Context("when adding a new route", func() {
			It("should add the gateways of the route to the state", func() {
				state.AddRoute(routeName1, gatewayNames)
				state.AddRoute(routeName2, gatewayNames)
				Expect(state.MatchingGatewaysByRoute(routeName1)).To(ContainElements(gatewayName1, gatewayName2))
				By("update", func() {
					state.AddRoute(routeName1, resources.NewObjectNameSet(gatewayName2))
					Expect(state.MatchingGatewaysByRoute(routeName1)).To(ContainElements(gatewayName2))
					Expect(state.MatchingGatewaysByRoute(routeName2)).To(ContainElements(gatewayName1, gatewayName2))
					state.AddRoute(routeName1, resources.NewObjectNameSet())
					Expect(state.MatchingGatewaysByRoute(routeName1)).To(BeEmpty())
					state.AddRoute(routeName1, resources.NewObjectNameSet(gatewayName1))
					Expect(state.MatchingGatewaysByRoute(routeName1)).To(ContainElements(gatewayName1))
					Expect(state.MatchingGatewaysByRoute(routeName2)).To(ContainElements(gatewayName1, gatewayName2))
				})
			})
		})
	})

	Describe("RemoveRoute", func() {
		Context("when removing an existing route", func() {
			BeforeEach(func() {
				state.AddRoute(routeName1, gatewayNames)
				state.AddRoute(routeName2, gatewayNames)
			})

			It("should remove the gateways from the state", func() {
				state.RemoveRoute(routeName1)
				state.RemoveRoute(routeName1)
				Expect(state.MatchingGatewaysByRoute(routeName1)).To(BeEmpty())
				state.RemoveRoute(routeName2)
				Expect(state.MatchingGatewaysByRoute(routeName2)).To(BeEmpty())
				Expect(state.RoutesCount()).To(Equal(0))
			})
		})
	})
})
