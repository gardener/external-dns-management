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

package resources

import (
	"github.com/gardener/controller-manager-library/pkg/resources/abstract"
)

type ObjectUpdater interface {
	Update() error
}

type ObjectStatusUpdater interface {
	UpdateStatus() error
}

type ModificationUpdater interface {
	IsModified() bool
	ObjectUpdater
}

type ModificationStatusUpdater interface {
	IsModified() bool
	ObjectStatusUpdater
}

type ModificationState struct {
	*abstract.ModificationState
}

var _ ModificationUpdater = (*ModificationState)(nil)

func NewModificationState(object Object, mod ...bool) *ModificationState {
	return &ModificationState{abstract.NewModificationState(object, mod...)}
}

func (this *ModificationState) Object() Object {
	return this.ModificationState.Object().(Object)
}

func (this *ModificationState) Update() error {
	if this.Modified {
		return this.Object().Update()
	}
	return nil
}

func (this *ModificationState) UpdateStatus() error {
	if this.Modified {
		return this.Object().UpdateStatus()
	}
	return nil
}

func (this *ModificationState) AssureLabel(name, value string) *ModificationState {
	this.ModificationState.AssureLabel(name, value)
	return this
}

func (this *ModificationState) AddOwners(objs ...Object) *ModificationState {
	for _, o := range objs {
		if this.Object().AddOwner(o) {
			this.Modified = true
		}
	}
	return this
}

////////////////////////////////////////////////////////////////////////////////

func ModifyStatus(obj Object, f func(*ModificationState) error) (bool, error) {
	m := func(data ObjectData) (bool, error) {
		o, err := obj.Resources().Wrap(data)
		if err != nil {
			return false, err
		}
		mod := NewModificationState(o)
		err = f(mod)
		return mod.Modified, err
	}
	return obj.ModifyStatus(m)
}

func Modify(obj Object, f func(*ModificationState) error) (bool, error) {
	m := func(data ObjectData) (bool, error) {
		o, err := obj.Resources().Wrap(data)
		if err != nil {
			return false, err
		}
		mod := NewModificationState(o)
		err = f(mod)
		return mod.Modified, err
	}
	return obj.Modify(m)
}

func CreateOrModify(obj Object, f func(*ModificationState) error) (bool, error) {
	m := func(data ObjectData) (bool, error) {
		o, err := obj.Resources().Wrap(data)
		if err != nil {
			return false, err
		}
		mod := NewModificationState(o)
		err = f(mod)
		return mod.Modified, err
	}
	return obj.CreateOrModify(m)
}
