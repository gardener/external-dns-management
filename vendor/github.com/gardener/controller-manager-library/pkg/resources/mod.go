/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package resources

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/fieldpath"
	"github.com/gardener/controller-manager-library/pkg/logger"
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

func NewModificationState(object Object, settings ...interface{}) *ModificationState {
	return &ModificationState{abstract.NewModificationState(object, settings...)}
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

type ModificationStateUpdater func(*ModificationState) error

type updater struct {
	obj      Object
	funcs    []ModificationStateUpdater
	modified bool
}

var _ ModificationStatusUpdater = (*updater)(nil)
var _ ModificationUpdater = (*updater)(nil)

func NewUpdater(obj Object, funcs ...ModificationStateUpdater) *updater {
	return &updater{
		obj:   obj,
		funcs: funcs,
	}
}

func (this *updater) UpdateStatus() error {
	mod, err := ModifyStatus(this.obj, this.funcs...)
	this.modified = mod
	return err
}

func (this *updater) Update() error {
	mod, err := Modify(this.obj, this.funcs...)
	this.modified = mod
	return err
}

func (this *updater) IsModified() bool {
	return this.modified
}

////////////////////////////////////////////////////////////////////////////////

var pState = fieldpath.MustFieldPath(".Status.State")
var pMessage = fieldpath.MustFieldPath(".Status.Message")

func UpdateStandardObjectStatus(log logger.LogContext, obj Object, state, msg string) (bool, error) {
	return ModifyStatus(obj, StandardObjectStatusUpdater(log, state, msg))
}

func StandardObjectStatusUpdater(log logger.LogContext, state, msg string) ModificationStateUpdater {
	return func(mod *ModificationState) error {
		mod.Set(pState, state)
		mod.Set(pMessage, msg)
		if log != nil && mod.IsModified() {
			log.Infof("updating state %s (%s)", state, msg)
		}
		return nil
	}
}

func UpdateStandardObjectStatusf(log logger.LogContext, obj Object, state, msg string, args ...interface{}) (bool, error) {
	return UpdateStandardObjectStatus(log, obj, state, fmt.Sprintf(msg, args...))
}

func NewStandardStatusUpdate(log logger.LogContext, obj Object, state, msg string) ModificationStatusUpdater {
	return NewUpdater(obj, func(mod *ModificationState) error {
		mod.Set(pState, state)
		mod.Set(pMessage, msg)
		if log != nil && mod.IsModified() {
			log.Infof("updating state %s (%s)", state, msg)
		}
		return nil
	})
}

func NewStandardStatusUpdatef(log logger.LogContext, obj Object, state, msg string, args ...interface{}) ModificationStatusUpdater {
	return NewStandardStatusUpdate(log, obj, state, fmt.Sprintf(msg, args...))
}

////////////////////////////////////////////////////////////////////////////////

func ModifyStatus(obj Object, funcs ...ModificationStateUpdater) (bool, error) {
	m := func(data ObjectData) (bool, error) {
		o, err := obj.Resources().Wrap(data)
		if err != nil {
			return false, err
		}
		mod := NewModificationState(o)
		for _, f := range funcs {
			err = f(mod)
			if err != nil {
				break
			}
		}
		return mod.Modified, err
	}
	return obj.ModifyStatus(m)
}

func Modify(obj Object, funcs ...ModificationStateUpdater) (bool, error) {
	m := func(data ObjectData) (bool, error) {
		o, err := obj.Resources().Wrap(data)
		if err != nil {
			return false, err
		}
		mod := NewModificationState(o)
		for _, f := range funcs {
			err = f(mod)
			if err != nil {
				break
			}
		}
		return mod.Modified, err
	}
	return obj.Modify(m)
}

func CreateOrModify(obj Object, f ModificationStateUpdater) (bool, error) {
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
