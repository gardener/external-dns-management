/*
 * SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 *
 */

package abstract

import (
	"reflect"

	"github.com/gardener/controller-manager-library/pkg/fieldpath"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources/conditions"
	"github.com/gardener/controller-manager-library/pkg/utils"
)

type ModificationState struct {
	utils.ModificationState
	object  Object
	handler conditions.ModificationHandler
	logger.LogContext
}

func NewModificationState(object Object, settings ...interface{}) *ModificationState {
	var log logger.LogContext
	aggr := false
	for _, s := range settings {
		switch v := s.(type) {
		case bool:
			aggr = aggr || v
		case logger.LogContext:
			log = v
		default:
			return nil
		}
	}
	if log == nil {
		log = logger.New()
	}
	s := &ModificationState{utils.ModificationState{aggr}, object, nil, log}
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

func (this *ModificationState) Get(field fieldpath.Path) (interface{}, error) {
	return field.Get(this.object.Data())
}

func (this *ModificationState) Set(field fieldpath.Path, value interface{}) error {
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
