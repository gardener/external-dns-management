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
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
)

type Finalizer interface {
	FinalizerName(obj resources.Object) string
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
	return obj.HasFinalizer(this.FinalizerName(obj))
}

func (this *DefaultFinalizer) SetFinalizer(obj resources.Object) error {
	return obj.SetFinalizer(this.FinalizerName(obj))
}

func (this *DefaultFinalizer) RemoveFinalizer(obj resources.Object) error {
	return obj.RemoveFinalizer(this.FinalizerName(obj))
}

func (this *DefaultFinalizer) FinalizerName(obj resources.Object) string {
	return this.name
}

///////////////////////////////////////////////////////////////////////////////

type ClassesFinalizer struct {
	base    string
	classes *Classes
}

func NewFinalizerForClasses(logger logger.LogContext, name string, classes *Classes) Finalizer {
	this := ClassesFinalizer{name, classes}
	n := this.finalizer()
	if n != name {
		logger.Infof("switching finalizer to %q", n)
	}
	if classes.Size() == 1 {
		return NewDefaultFinalizer(n)
	}
	return &this

}

func (this *ClassesFinalizer) finalizer(eff ...string) string {
	class := this.classes.Main()
	if len(eff) > 0 {
		class = eff[0]
	}
	if class == this.classes.Default() {
		return this.base
	}
	return class + "." + this.base
}

func (this *ClassesFinalizer) HasFinalizer(obj resources.Object) bool {
	for c := range this.classes.Classes() {
		if obj.HasFinalizer(this.finalizer(c)) {
			return true
		}
	}
	return false
}

func (this *ClassesFinalizer) FinalizerName(obj resources.Object) string {
	return this.finalizer()
}

func (this *ClassesFinalizer) SetFinalizer(obj resources.Object) error {
	err := obj.SetFinalizer(this.FinalizerName(obj))
	if err != nil {
		return err
	}
	for c := range this.classes.Classes() {
		if c != this.classes.Main() {
			err = obj.RemoveFinalizer(this.finalizer(c))
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (this *ClassesFinalizer) RemoveFinalizer(obj resources.Object) error {
	for c := range this.classes.Classes() {
		err := obj.RemoveFinalizer(this.finalizer(c))
		if err != nil {
			return err
		}
	}
	return nil
}
