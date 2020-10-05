/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package resources

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/resources/abstract"
)

type AbstractObject struct {
	*abstract.AbstractObject
	self I_Object
}

func NewAbstractObject(self I_Object, data ObjectData, r Interface) AbstractObject {
	return AbstractObject{abstract.NewAbstractObject(data, r), self}
}

func (this *AbstractObject) GetResource() Interface {
	return this.AbstractObject.GetResource().(Interface)
}

func (this *AbstractObject) GetCluster() Cluster {
	return this.GetResource().GetCluster()
}

func (this *AbstractObject) ClusterKey() ClusterObjectKey {
	return NewClusterKey(this.GetResource().GetCluster().GetId(), this.GroupKind(), this.GetNamespace(), this.GetName())
}

func (this *AbstractObject) Description() string {
	return fmt.Sprintf("%s:%s", this.GetCluster().GetId(), this.AbstractObject.Description())
}

func (this *AbstractObject) IsCoLocatedTo(o Object) bool {
	if o == nil {
		return true
	}
	return o.GetCluster() == this.GetCluster()
}

func (this *AbstractObject) Resources() Resources {
	return this.GetResource().Resources()
}

func (this *AbstractObject) Event(eventtype, reason, message string) {
	this.Resources().Event(this.ObjectData, eventtype, reason, message)
}

func (this *AbstractObject) Eventf(eventtype, reason, messageFmt string, args ...interface{}) {
	this.Resources().Eventf(this.ObjectData, eventtype, reason, messageFmt, args...)
}

func (this *AbstractObject) AnnotatedEventf(annotations map[string]string, eventtype, reason, messageFmt string, args ...interface{}) {
	this.Resources().AnnotatedEventf(this.ObjectData, annotations, eventtype, reason, messageFmt, args...)
}
