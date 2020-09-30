/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package resources

import (
	"reflect"

	"github.com/gardener/controller-manager-library/pkg/utils"
)

var cluster_key reflect.Type

func init() {
	cluster_key, _ = utils.TypeKey((*Cluster)(nil))
}

// _object is the standard implementation of the Object interface
// it uses the AbstractObject as base to provide standard implementations
// based on the internal object interface. (see _i_object)
type _object struct {
	AbstractObject
	resource Internal
}

var _ Object = &_object{}

func newObject(data ObjectData, resource Internal) Object {
	o := &_object{AbstractObject{}, resource}
	o.AbstractObject = NewAbstractObject(&_i_object{o}, data, resource.Resource())
	return o
}

func (this *_object) DeepCopy() Object {
	data := this.ObjectData.DeepCopyObject().(ObjectData)
	return newObject(data, this.resource)
}
