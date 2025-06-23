// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

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
