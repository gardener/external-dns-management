/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 */

package minimal

import (
	"reflect"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
)

type MinimalMeta struct {
	Name              string            `json:"name,omitempty"`
	Namespace         string            `json:"namespace,omitempty"`
	UID               types.UID         `json:"uid,omitempty"`
	ResourceVersion   string            `json:"resourceVersion,omitempty"`
	Generation        int64             `json:"generation,omitempty"`
	CreationTimestamp metav1.Time       `json:"creationTimestamp,omitempty"`
	DeletionTimestamp *metav1.Time      `json:"deletionTimestamp,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
}

type MinimalObject struct {
	metav1.TypeMeta `json:",inline"`
	MinimalMeta     `json:"metadata,omitempty"`
}

var _ runtime.Object = &MinimalObject{}
var _ metav1.Object = &MinimalObject{}

var minimalObjectType = reflect.TypeOf(MinimalObject{})

func (o *MinimalObject) DeepCopyObject() runtime.Object {
	n := *o
	return &n
}
func (o *MinimalObject) DeepCopyInto(out *MinimalObject) {
	*out = *o
}

func (o *MinimalObject) GetNamespace() string {
	return o.Namespace
}
func (o *MinimalObject) SetNamespace(namespace string) {
	o.Namespace = namespace
}
func (o *MinimalObject) GetName() string {
	return o.Name
}
func (o *MinimalObject) SetName(name string) {
	o.Name = name
}
func (o *MinimalObject) GetGenerateName() string {
	o.unsupported()
	return ""
}
func (o *MinimalObject) SetGenerateName(name string) {
	o.unsupported()
}
func (o *MinimalObject) GetUID() types.UID {
	return o.UID
}
func (o *MinimalObject) SetUID(uid types.UID) {
	o.UID = uid
}
func (o *MinimalObject) GetResourceVersion() string {
	return o.ResourceVersion
}
func (o *MinimalObject) SetResourceVersion(version string) {
	o.ResourceVersion = version
}
func (o *MinimalObject) GetGeneration() int64 {
	return o.Generation
}
func (o *MinimalObject) SetGeneration(generation int64) {
	o.Generation = generation
}
func (o *MinimalObject) GetSelfLink() string {
	o.unsupported()
	return ""
}
func (o *MinimalObject) SetSelfLink(selfLink string) {
	o.unsupported()
}
func (o *MinimalObject) GetCreationTimestamp() metav1.Time {
	return o.CreationTimestamp
}
func (o *MinimalObject) SetCreationTimestamp(timestamp metav1.Time) {
	o.CreationTimestamp = timestamp
}
func (o *MinimalObject) GetDeletionTimestamp() *metav1.Time {
	return o.DeletionTimestamp
}
func (o *MinimalObject) SetDeletionTimestamp(timestamp *metav1.Time) {
	o.DeletionTimestamp = timestamp
}
func (o *MinimalObject) GetDeletionGracePeriodSeconds() *int64 {
	o.unsupported()
	return nil
}
func (o *MinimalObject) SetDeletionGracePeriodSeconds(*int64) {
	o.unsupported()
}
func (o *MinimalObject) GetLabels() map[string]string {
	return o.Labels
}
func (o *MinimalObject) SetLabels(labels map[string]string) {
	o.Labels = labels
}
func (o *MinimalObject) GetAnnotations() map[string]string {
	o.unsupported()
	return nil
}
func (o *MinimalObject) SetAnnotations(annotations map[string]string) {
	o.unsupported()
}
func (o *MinimalObject) GetFinalizers() []string {
	o.unsupported()
	return nil
}
func (o *MinimalObject) SetFinalizers(finalizers []string) {
	o.unsupported()
}
func (o *MinimalObject) GetOwnerReferences() []metav1.OwnerReference {
	o.unsupported()
	return nil
}
func (o *MinimalObject) SetOwnerReferences([]metav1.OwnerReference) {
	o.unsupported()
}
func (o *MinimalObject) GetClusterName() string {
	o.unsupported()
	return ""
}
func (o *MinimalObject) SetClusterName(clusterName string) {
	o.unsupported()
}
func (o *MinimalObject) GetManagedFields() []metav1.ManagedFieldsEntry {
	o.unsupported()
	return nil
}
func (o *MinimalObject) SetManagedFields(managedFields []metav1.ManagedFieldsEntry) {
	o.unsupported()
}

func (o *MinimalObject) unsupported() {
	panic("unsupported")
}
