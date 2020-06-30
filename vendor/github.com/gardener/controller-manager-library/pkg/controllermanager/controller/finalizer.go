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
	"github.com/gardener/controller-manager-library/pkg/utils"
)

type Finalizer interface {
	FinalizerName(obj resources.Object) string
	HasFinalizer(obj resources.Object) bool
	SetFinalizer(obj resources.Object) error
	RemoveFinalizer(obj resources.Object) error
}

type FinalizerGroup interface {
	Finalizer
	Main() string
	Finalizers() utils.StringSet
}

type NameMapper func(name ...string) string

type DefaultFinalizer struct {
	name string
}

func NewDefaultFinalizer(name string) FinalizerGroup {
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

func (this *DefaultFinalizer) Main() string {
	return this.name
}

func (this *DefaultFinalizer) Finalizers() utils.StringSet {
	return utils.NewStringSet(this.name)
}

///////////////////////////////////////////////////////////////////////////////

type DefaultFinalizerGroup struct {
	main string
	set  utils.StringSet
}

func NewFinalizerGroup(name string, set utils.StringSet, mapper ...NameMapper) FinalizerGroup {
	set.Add(name)
	this := DefaultFinalizerGroup{name, set}
	if len(set) == 1 {
		return NewDefaultFinalizer(name)
	}
	return &this

}

func (this *DefaultFinalizerGroup) HasFinalizer(obj resources.Object) bool {
	for c := range this.set {
		if obj.HasFinalizer(c) {
			return true
		}
	}
	return false
}

func (this *DefaultFinalizerGroup) FinalizerName(obj resources.Object) string {
	return this.main
}

func (this *DefaultFinalizerGroup) SetFinalizer(obj resources.Object) error {
	if obj.IsDeleting() {
		// to avoid finalizer migration during deletion
		// the new finalizer is NOT added, but the old ones are still kept.
		// they will be removed later by the corresponding call to RemoveFinalizer
		return nil
	}
	err := obj.SetFinalizer(this.FinalizerName(obj))
	if err != nil {
		return err
	}
	for c := range this.set {
		if c != this.main {
			err = obj.RemoveFinalizer(c)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (this *DefaultFinalizerGroup) RemoveFinalizer(obj resources.Object) error {
	for c := range this.set {
		err := obj.RemoveFinalizer(c)
		if err != nil {
			return err
		}
	}
	return nil
}

func (this *DefaultFinalizerGroup) Main() string {
	return this.main
}

func (this *DefaultFinalizerGroup) Finalizers() utils.StringSet {
	return utils.NewStringSetBySets(this.set)
}

///////////////////////////////////////////////////////////////////////////////

func NewFinalizerForGroupAndClasses(group FinalizerGroup, classes *Classes) FinalizerGroup {
	set := classesFinalizerSet(group, classes)
	main := classFinalizer(group.Main(), classes)
	return NewFinalizerGroup(main, set)
}

func classFinalizer(base string, classes *Classes, class ...string) string {
	classname := classes.Main()
	if len(class) > 0 {
		classname = class[0]
	}
	if classname == classes.Default() {
		return base
	}
	return classname + "." + base
}

func classFinalizers(group FinalizerGroup, classes *Classes, class string) utils.StringSet {
	set := utils.StringSet{}

	for b := range group.Finalizers() {
		set.Add(classFinalizer(b, classes, class))
	}
	return set
}

func classesFinalizerSet(group FinalizerGroup, classes *Classes) utils.StringSet {
	set := utils.StringSet{}

	for c := range classes.classes {
		set.AddSet(classFinalizers(group, classes, c))
	}
	return set
}

////////////////////////////////////////////////////////////////////////////////

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
	return classFinalizer(this.base, this.classes, eff...)
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
	if obj.IsDeleting() {
		// to avoid finalizer migration during deletion
		// the new finalizer is NOT added, but the old ones are still kept.
		// they will be removed later by the corresponding call to RemoveFinalizer
		return nil
	}
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
