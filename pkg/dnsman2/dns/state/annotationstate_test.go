// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package state

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
)

var _ = Describe("AnnotationState", func() {
	var (
		s        *annotationState
		annotRef = client.ObjectKey{Namespace: "ns", Name: "annot"}
		refOld   = v1alpha1.ResourceReference{APIVersion: "v1", Kind: "Service", Namespace: "a", Name: "svc"}
		refNew   = v1alpha1.ResourceReference{APIVersion: "v1", Kind: "Service", Namespace: "b", Name: "svc"}
	)

	BeforeEach(func() {
		s = newAnnotationState()
	})

	It("does not leak a stale entry when resourceRef changes", func() {
		Expect(s.SetResourceAnnotations(refOld, annotRef, map[string]string{"k": "v"})).To(Succeed())
		Expect(s.SetResourceAnnotations(refNew, annotRef, map[string]string{"k": "v"})).To(Succeed())

		Expect(s.resourceAnnotations).To(HaveLen(1))
		_, _, activeOld := s.GetResourceAnnotationStatus(refOld)
		Expect(activeOld).To(BeFalse())
	})

	It("removes all entries owned by an annotation key", func() {
		Expect(s.SetResourceAnnotations(refOld, annotRef, nil)).To(Succeed())
		// force a second entry under the same annotationRef (pre-fix leak shape)
		s.resourceAnnotations[resourceRefToString(refNew)] = &annotationData{resourceRef: refNew, annotationRef: annotRef}

		s.DeleteByAnnotationKey(annotRef)
		Expect(s.resourceAnnotations).To(BeEmpty())
	})
})
