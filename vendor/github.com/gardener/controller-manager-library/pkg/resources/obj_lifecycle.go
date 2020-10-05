/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package resources

import (
	"github.com/gardener/controller-manager-library/pkg/resources/errors"
)

func (this *AbstractObject) Create() error {
	o, err := this.self.GetResource().Create(this.ObjectData)
	if err == nil {
		this.ObjectData = o.Data()
	}
	return err
}

func (this *AbstractObject) CreateOrUpdate() error {
	o, err := this.self.GetResource().CreateOrUpdate(this.ObjectData)
	if err == nil {
		this.ObjectData = o.Data()
	}
	return err
}

func (this *AbstractObject) IsDeleting() bool {
	return this.GetDeletionTimestamp() != nil
}

func (this *AbstractObject) Modify(modifier Modifier) (bool, error) {
	return this.modify(false, modifier)
}

func (this *AbstractObject) ModifyStatus(modifier Modifier) (bool, error) {
	return this.modifyStatus(modifier)
}

func (this *AbstractObject) CreateOrModify(modifier Modifier) (bool, error) {
	return this.modify(true, modifier)
}

func (this *AbstractObject) modifyStatus(modifier Modifier) (bool, error) {
	return this.self.I_modify(true, false, modifier)
}

func (this *AbstractObject) modify(create bool, modifier Modifier) (bool, error) {
	return this.self.I_modify(false, create, modifier)
}

////////////////////////////////////////////////////////////////////////////////
// Methods using internal Resource Interface

func (this *AbstractObject) Update() error {
	result, err := this.self.I_resource().I_update(this.ObjectData)
	if err == nil {
		this.ObjectData = result
	}
	return err
}

func (this *AbstractObject) UpdateStatus() error {
	rsc := this.self.I_resource()
	if !rsc.Info().HasStatusSubResource() {
		return errors.ErrNoStatusSubResource.New(rsc.GroupVersionKind())
	}
	result, err := rsc.I_updateStatus(this.ObjectData)
	if err == nil {
		this.ObjectData = result
	}
	return err
}

func (this *AbstractObject) Delete() error {
	return this.self.I_resource().I_delete(this)
}

func (this *AbstractObject) UpdateFromCache() error {
	obj, err := this.self.GetResource().GetCached(this.ObjectName())
	if err == nil {
		this.ObjectData = obj.Data()
	}
	return err
}
