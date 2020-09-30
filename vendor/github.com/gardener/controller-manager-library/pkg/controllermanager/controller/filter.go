/*
 * SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 *
 */

package controller

import "github.com/gardener/controller-manager-library/pkg/resources"

// ResourceFilter is the signature for filter implementations used to filter
// watched resources prior to putting it into a work queue. Objects
// reported false by a filter will be rejected
type ResourceFilter func(owning ResourceKey, resc resources.Object) bool

// Or can be used to generate a filter that reqports true if one of the
// given filters report true, If no filter is given always false is reported.
func Or(filters ...ResourceFilter) ResourceFilter {
	return func(owning ResourceKey, resc resources.Object) bool {
		for _, f := range filters {
			if f(owning, resc) {
				return true
			}
		}
		return false
	}
}

// And can be used to generate a filter that reqports true if all of the
// given filters report true, If no filter is given always false is reported.
func And(filters ...ResourceFilter) ResourceFilter {
	return func(owning ResourceKey, resc resources.Object) bool {
		for _, f := range filters {
			if !f(owning, resc) {
				return false
			}
		}
		return len(filters) > 0
	}
}
