/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package resources

import (
	"context"
	"fmt"
	"reflect"
	"sync"

	"golang.org/x/sync/semaphore"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"

	rerrors "github.com/gardener/controller-manager-library/pkg/resources/errors"
	"github.com/gardener/controller-manager-library/pkg/utils"
)

func SetAnnotation(o ObjectData, key, value string) bool {
	annos := o.GetAnnotations()
	if annos == nil {
		annos = map[string]string{}
	}
	old, ok := annos[key]
	if !ok || old != value {
		annos[key] = value
		o.SetAnnotations(annos)
		return true
	}
	return false
}

func RemoveAnnotation(o ObjectData, key string) bool {
	annos := o.GetAnnotations()
	if annos != nil {
		if _, ok := annos[key]; ok {
			delete(annos, key)
			o.SetAnnotations(annos)
			return true
		}
	}
	return false
}

func GetAnnotation(o ObjectData, key string) (string, bool) {
	annos := o.GetAnnotations()
	if annos == nil {
		return "", false
	}
	value, ok := annos[key]
	return value, ok
}

///////////////

func SetLabel(o ObjectData, key, value string) bool {
	labels := o.GetLabels()
	if labels == nil {
		labels = map[string]string{}
	}
	old, ok := labels[key]
	if !ok || old != value {
		labels[key] = value
		o.SetLabels(labels)
		return true
	}
	return false
}

func RemoveLabel(o ObjectData, key string) bool {
	labels := o.GetLabels()
	if labels != nil {
		if _, ok := labels[key]; ok {
			delete(labels, key)
			o.SetLabels(labels)
			return true
		}
	}
	return false
}

func GetLabel(o ObjectData, key string) (string, bool) {
	labels := o.GetLabels()
	if labels == nil {
		return "", false
	}
	value, ok := labels[key]
	return value, ok
}

//////////////

func SetOwnerReference(o ObjectData, ref *metav1.OwnerReference) bool {
	refs := o.GetOwnerReferences()
	for _, r := range refs {
		if r.UID == ref.UID {
			return false
		}
	}
	refs = append(refs, *ref)
	o.SetOwnerReferences(refs)
	return true
}

func getField(o ObjectData, name string) (interface{}, bool) {
	if utils.IsNil(o) {
		return nil, false
	}
	v := reflect.ValueOf(o)
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil, false
	}
	f := v.FieldByName(name)
	if f.IsZero() {
		return nil, false
	}
	if f.Kind() == reflect.Struct {
		return f.Addr().Interface(), true
	}
	if f.Kind() == reflect.Ptr {
		return f.Interface(), true
	}
	return f.Interface(), false
}

func GetObjectSpec(o ObjectData) (interface{}, bool) {
	return getField(o, "Spec")
}

func GetObjectStatus(o ObjectData) (interface{}, bool) {
	return getField(o, "Status")
}

func setField(o ObjectData, name string, value interface{}) error {
	if utils.IsNil(o) {
		return rerrors.New(rerrors.ERR_INVALID, "no object given")
	}
	v := reflect.ValueOf(o)
	for v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return rerrors.New(rerrors.ERR_INVALID, "no struct given")
	}
	f := v.FieldByName(name)
	if f.IsZero() {
		return rerrors.New(rerrors.ERR_INVALID, "invalid field %q", name)
	}
	if !f.CanSet() {
		return rerrors.New(rerrors.ERR_INVALID, "cannot set field %q for type %T", name, o)
	}
	tv := reflect.ValueOf(value)
	for tv.Kind() == reflect.Ptr && f.Kind() != reflect.Ptr {
		tv = tv.Elem()
	}
	if tv.Type() != f.Type() {
		if !tv.Type().ConvertibleTo(f.Type()) {
			return rerrors.New(rerrors.ERR_INVALID, "cannot set field %q for type %T: invalid type %T", name, o, value)
		}
		tv = tv.Convert(f.Type())
	}
	f.Set(tv)
	return nil
}

func SetObjectSpec(obj ObjectData, value interface{}) error {
	return setField(obj, "Spec", value)
}

func RemoveOwnerReference(o ObjectData, ref *metav1.OwnerReference) bool {
	refs := o.GetOwnerReferences()
	for i, r := range refs {
		if r.UID == ref.UID {
			refs = append(refs[:i], refs[i+1:]...)
			o.SetOwnerReferences(refs)
			return true
		}
	}
	return false
}

func FilterKeysByGroupKinds(keys ClusterObjectKeySet, kinds ...schema.GroupKind) ClusterObjectKeySet {

	if len(kinds) == 0 {
		return keys.Copy()
	}
	set := ClusterObjectKeySet{}
outer:
	for k := range keys {
		for _, g := range kinds {
			if k.GroupKind() == g {
				set.Add(k)
				continue outer
			}
		}
	}
	return set
}

func ObjectArrayToString(objs ...Object) string {
	s := "["
	sep := ""
	for _, o := range objs {
		s = fmt.Sprintf("%s%s%s", s, sep, o.ClusterKey())
		sep = ", "
	}
	return s + "]"
}

func AddLabel(labels map[string]string, key, value string) map[string]string {
	new := map[string]string{}
	for k, v := range labels {
		new[k] = v
	}
	new[key] = value
	return new
}

func IsObjectDeletionError(err error) bool {
	return FilterObjectDeletionError(err) != nil
}

func FilterObjectDeletionError(args ...interface{}) error {
	if len(args) == 0 {
		return nil
	}
	if err, ok := args[len(args)-1].(error); ok {
		if err == nil || errors.IsNotFound(err) {
			return nil
		}
		return err
	}
	return nil
}

////////////////////////////////////////////////////////////////////////////////

type ClusterObjectKeyLocks struct {
	lock  sync.Mutex
	locks map[ClusterObjectKey]*semaphore.Weighted
}

func (this *ClusterObjectKeyLocks) TryLock(key ClusterObjectKey) bool {
	var lock *semaphore.Weighted
	this.lock.Lock()

	if this.locks == nil {
		this.locks = map[ClusterObjectKey]*semaphore.Weighted{}
	} else {
		lock = this.locks[key]
	}
	if lock == nil {
		lock = semaphore.NewWeighted(1)
		this.locks[key] = lock
	}
	this.lock.Unlock()
	return lock.TryAcquire(1)
}

func (this *ClusterObjectKeyLocks) Lock(ctx context.Context, key ClusterObjectKey) error {
	var lock *semaphore.Weighted
	if ctx == nil {
		ctx = context.Background()
	}
	this.lock.Lock()

	if this.locks == nil {
		this.locks = map[ClusterObjectKey]*semaphore.Weighted{}
	} else {
		lock = this.locks[key]
	}
	if lock == nil {
		lock = semaphore.NewWeighted(1)
		this.locks[key] = lock
	}
	this.lock.Unlock()
	return lock.Acquire(ctx, 1)
}

func (this *ClusterObjectKeyLocks) Unlock(key ClusterObjectKey) {
	var lock *semaphore.Weighted
	this.lock.Lock()

	if this.locks != nil {
		lock = this.locks[key]
		if lock != nil {
			lock.Release(1)
			if lock.TryAcquire(1) {
				delete(this.locks, key)
			}
		}
	}
	this.lock.Unlock()
}
