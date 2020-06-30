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

package abstract

import (
	"reflect"

	"github.com/gardener/controller-manager-library/pkg/fieldpath"
	"github.com/gardener/controller-manager-library/pkg/resources/conditions"
	"github.com/gardener/controller-manager-library/pkg/utils"
)

type ModificationState struct {
	utils.ModificationState
	object  Object
	handler conditions.ModificationHandler
}

func NewModificationState(object Object, mod ...bool) *ModificationState {
	aggr := false
	for _, m := range mod {
		aggr = aggr || m
	}
	s := &ModificationState{utils.ModificationState{aggr}, object, nil}
	s.handler = &modhandler{s}
	return s
}

func (this *ModificationState) Object() Object {
	return this.object
}

func (this *ModificationState) Data() ObjectData {
	return this.object.Data()
}

func (this *ModificationState) Condition(t *conditions.ConditionType) *conditions.Condition {
	c := t.Get(this.Data())
	if c != nil {
		c.AddModificationHandler(this.handler)
	}
	return c
}

func (this *ModificationState) Conditions(layout *conditions.ConditionLayout) (*conditions.Conditions, error) {
	c, err := layout.For(this.Data())
	if err != nil {
		return nil, err
	}
	if c != nil {
		c.AddModificationHandler(this.handler)
	}
	return c, nil
}

func (this *ModificationState) Apply(f func(obj Object) bool) *ModificationState {
	this.Modified = this.Modified || f(this.object)
	return this
}

func (this *ModificationState) AssureLabel(name, value string) *ModificationState {
	labels := this.object.GetLabels()
	if labels[name] != value {
		if value == "" {
			delete(labels, name)
		} else {
			labels[name] = value
		}
		this.Modified = true
	}
	return this
}

func (this *ModificationState) Get(field fieldpath.Field) (interface{}, error) {
	return field.Get(this.object.Data())
}

func (this *ModificationState) Set(field fieldpath.Field, value interface{}) error {
	old, err := field.Get(this.object.Data())
	if err != nil {
		return err
	}
	if reflect.DeepEqual(old, value) {
		return nil
	}
	err = field.Set(this.object.Data(), value)
	if err != nil {
		return err
	}
	this.Modified = true
	return nil
}

type modhandler struct {
	state *ModificationState
}

func (this *modhandler) Modified(condition *conditions.Condition) {
	this.state.Modify(true)
}
