/*
 * SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 */

package resources

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// VersionedObjects is used by Decoders to give callers a way to access all versions
// of an object during the decoding process.
//
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +k8s:deepcopy-gen=true
type VersionedObjects struct {
	// Objects is the set of objects retrieved during decoding, in order of conversion.
	// The 0 index is the object as serialized on the wire. If conversion has occurred,
	// other objects may be present. The right most object is the same as would be returned
	// by a normal Decode call.
	Objects []runtime.Object
}

// DeepCopyObject returns a deep copy
func (obj *VersionedObjects) DeepCopyObject() runtime.Object {
	if obj == nil {
		return obj
	}
	r := &VersionedObjects{}
	if obj.Objects != nil {
		r.Objects = make([]runtime.Object, len(obj.Objects), len(obj.Objects))
		for i, o := range obj.Objects {
			r.Objects[i] = o.DeepCopyObject()
		}
	}
	return r
}

// GetObjectKind implements Object for VersionedObjects, returning an empty ObjectKind
// interface if no objects are provided, or the ObjectKind interface of the object in the
// highest array position.
func (obj *VersionedObjects) GetObjectKind() schema.ObjectKind {
	last := obj.Last()
	if last == nil {
		return schema.EmptyObjectKind
	}
	return last.GetObjectKind()
}

// First returns the leftmost object in the VersionedObjects array, which is usually the
// object as serialized on the wire.
func (obj *VersionedObjects) First() runtime.Object {
	if len(obj.Objects) == 0 {
		return nil
	}
	return obj.Objects[0]
}

// Last is the rightmost object in the VersionedObjects array, which is the object after
// all transformations have been applied. This is the same object that would be returned
// by Decode in a normal invocation (without VersionedObjects in the into argument).
func (obj *VersionedObjects) Last() runtime.Object {
	if len(obj.Objects) == 0 {
		return nil
	}
	return obj.Objects[len(obj.Objects)-1]
}
