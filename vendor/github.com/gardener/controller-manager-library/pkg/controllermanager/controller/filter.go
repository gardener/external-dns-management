/*
 * Copyright 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *       http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
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

// ANd can be used to generate a filter that reqports true if all of the
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
