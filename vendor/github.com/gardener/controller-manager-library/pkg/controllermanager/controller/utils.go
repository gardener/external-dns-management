/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 *
 */

package controller

import (
	"github.com/gardener/controller-manager-library/pkg/resources"
)

func (this *controller) HasFinalizer(obj resources.Object) bool {
	return obj.HasFinalizer(this.computeFinalizer(this.owning))
}

func (this *controller) SetFinalizer(obj resources.Object) error {
	return obj.SetFinalizer(this.computeFinalizer(this.owning))
}

func (this *controller) RemoveFinalizer(obj resources.Object) error {
	return obj.RemoveFinalizer(this.computeFinalizer(this.owning))
}

func (this *controller) computeFinalizer(key ResourceKey) string {
	return this.definition.FinalizerName()
	//return fmt.Sprintf("%s/%s", this._Definition.FinalizerPrefix(), strings.Replace(key.String(), "/", ".", -1))
}

func (this *controller) FinalizerName() string {
	return this.computeFinalizer(this.owning)
}

///////////////////////////////////////////////////////////////////////////////

type definition_field interface {
	Definition
}

type DefinitionWrapper struct {
	definition_field
	filters []ResourceFilter
}

func (this *DefinitionWrapper) Definition() Definition {
	return this
}

func (this *DefinitionWrapper) ResourceFilters() []ResourceFilter {
	return append(this.ResourceFilters(), this.filters...)
}

var _ Definition = &DefinitionWrapper{}

func AddFilters(def Definition, filters ...ResourceFilter) Definition {
	return &DefinitionWrapper{def, filters}
}
