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

import "github.com/gardener/controller-manager-library/pkg/resources"

type Finalizer interface {
	HasFinalizer(obj resources.Object) bool
	SetFinalizer(obj resources.Object) error
	RemoveFinalizer(obj resources.Object) error
}

type DefaultFinalizer struct {
	name string
}

func NewDefaultFinalizer(name string) Finalizer {
	return &DefaultFinalizer{name}
}

func (this *DefaultFinalizer) HasFinalizer(obj resources.Object) bool {
	return obj.HasFinalizer(this.FinalizerName())
}

func (this *DefaultFinalizer) SetFinalizer(obj resources.Object) error {
	return obj.SetFinalizer(this.FinalizerName())
}

func (this *DefaultFinalizer) RemoveFinalizer(obj resources.Object) error {
	return obj.RemoveFinalizer(this.FinalizerName())
}

func (this *DefaultFinalizer) FinalizerName() string {
	return this.name
}
