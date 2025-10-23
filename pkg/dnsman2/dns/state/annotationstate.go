// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package state

import (
	"context"
	"fmt"
	"sync"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
)

// AnnotationState manages the state of DNS annotations for resources.
type AnnotationState interface {
	// SetResourceAnnotations sets the annotations for a given resource reference.
	SetResourceAnnotations(ref v1alpha1.ResourceReference, annotationRef client.ObjectKey, annotations map[string]string) error
	// GetResourceAnnotationStatus retrieves the status message and active state for a given resource reference.
	GetResourceAnnotationStatus(ref v1alpha1.ResourceReference) (annotations map[string]string, message string, active bool)
	// DeleteResourceAnnotations removes the annotations for a given resource reference.
	DeleteResourceAnnotations(ref v1alpha1.ResourceReference)
	// DeleteByAnnotationKey removes the annotations associated with a given annotation object key.
	DeleteByAnnotationKey(annotationKey client.ObjectKey)
	// UpdateStatus updates the status message and active state for a given resource reference.
	UpdateStatus(ctx context.Context, c client.Client, ref v1alpha1.ResourceReference, active bool) error
	// Reset clears all stored annotations.
	Reset()
}

type annotationState struct {
	lock                sync.Mutex
	resourceAnnotations map[string]*annotationData
}

var _ AnnotationState = &annotationState{}

type annotationData struct {
	resourceRef   v1alpha1.ResourceReference
	annotations   map[string]string
	annotationRef client.ObjectKey
	message       string
	active        bool
}

func newAnnotationState() *annotationState {
	return &annotationState{
		resourceAnnotations: make(map[string]*annotationData),
	}
}

func (s *annotationState) SetResourceAnnotations(ref v1alpha1.ResourceReference, annotationRef client.ObjectKey, annotations map[string]string) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	key := resourceRefToString(ref)
	data := s.resourceAnnotations[key]
	if data != nil {
		// Check if the annotationRef matches
		if data.annotationRef != annotationRef {
			return fmt.Errorf("conflicting DNSAnnotation for the same resource reference: %s", data.annotationRef)
		}
	} else {
		data = &annotationData{
			resourceRef:   ref,
			annotations:   annotations,
			annotationRef: annotationRef,
		}
	}

	data.resourceRef = ref
	data.annotations = annotations
	data.annotationRef = annotationRef
	s.resourceAnnotations[key] = data

	return nil
}

func (s *annotationState) GetResourceAnnotationStatus(ref v1alpha1.ResourceReference) (annotations map[string]string, message string, active bool) {
	s.lock.Lock()
	defer s.lock.Unlock()

	key := resourceRefToString(ref)
	data := s.resourceAnnotations[key]
	if data != nil {
		return data.annotations, data.message, data.active
	}
	return nil, "", false
}

func (s *annotationState) DeleteResourceAnnotations(ref v1alpha1.ResourceReference) {
	s.lock.Lock()
	defer s.lock.Unlock()

	key := resourceRefToString(ref)
	delete(s.resourceAnnotations, key)
}

func (s *annotationState) DeleteByAnnotationKey(annotationKey client.ObjectKey) {
	s.lock.Lock()
	defer s.lock.Unlock()

	for key, data := range s.resourceAnnotations {
		if data.annotationRef == annotationKey {
			delete(s.resourceAnnotations, key)
			break
		}
	}
}

func (s *annotationState) Reset() {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.resourceAnnotations = make(map[string]*annotationData)
}

func (s *annotationState) UpdateStatus(ctx context.Context, c client.Client, ref v1alpha1.ResourceReference, active bool) error {
	message := ""
	key := s.updateStatusInternal(ref, message, active)
	if key == nil {
		return nil
	}
	obj := v1alpha1.DNSAnnotation{}
	if err := c.Get(ctx, *key, &obj); err != nil {
		return err
	}
	patch := client.MergeFrom(obj.DeepCopy())
	obj.Status.Message = message
	obj.Status.Active = active
	return c.Status().Patch(ctx, &obj, patch)
}

func (s *annotationState) updateStatusInternal(ref v1alpha1.ResourceReference, message string, active bool) *client.ObjectKey {
	s.lock.Lock()
	defer s.lock.Unlock()

	key := resourceRefToString(ref)
	data := s.resourceAnnotations[key]
	if data == nil {
		return nil
	}

	data.message = message
	data.active = active
	return &data.annotationRef
}

func resourceRefToString(ref v1alpha1.ResourceReference) string {
	return ref.APIVersion + "/" + ref.Kind + "/" + ref.Namespace + "/" + ref.Name
}
