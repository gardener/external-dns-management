// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0
// Code generated by lister-gen. DO NOT EDIT.

package v1alpha1

import (
	v1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

// DNSAnnotationLister helps list DNSAnnotations.
// All objects returned here must be treated as read-only.
type DNSAnnotationLister interface {
	// List lists all DNSAnnotations in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha1.DNSAnnotation, err error)
	// DNSAnnotations returns an object that can list and get DNSAnnotations.
	DNSAnnotations(namespace string) DNSAnnotationNamespaceLister
	DNSAnnotationListerExpansion
}

// dNSAnnotationLister implements the DNSAnnotationLister interface.
type dNSAnnotationLister struct {
	indexer cache.Indexer
}

// NewDNSAnnotationLister returns a new DNSAnnotationLister.
func NewDNSAnnotationLister(indexer cache.Indexer) DNSAnnotationLister {
	return &dNSAnnotationLister{indexer: indexer}
}

// List lists all DNSAnnotations in the indexer.
func (s *dNSAnnotationLister) List(selector labels.Selector) (ret []*v1alpha1.DNSAnnotation, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.DNSAnnotation))
	})
	return ret, err
}

// DNSAnnotations returns an object that can list and get DNSAnnotations.
func (s *dNSAnnotationLister) DNSAnnotations(namespace string) DNSAnnotationNamespaceLister {
	return dNSAnnotationNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// DNSAnnotationNamespaceLister helps list and get DNSAnnotations.
// All objects returned here must be treated as read-only.
type DNSAnnotationNamespaceLister interface {
	// List lists all DNSAnnotations in the indexer for a given namespace.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha1.DNSAnnotation, err error)
	// Get retrieves the DNSAnnotation from the indexer for a given namespace and name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1alpha1.DNSAnnotation, error)
	DNSAnnotationNamespaceListerExpansion
}

// dNSAnnotationNamespaceLister implements the DNSAnnotationNamespaceLister
// interface.
type dNSAnnotationNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all DNSAnnotations in the indexer for a given namespace.
func (s dNSAnnotationNamespaceLister) List(selector labels.Selector) (ret []*v1alpha1.DNSAnnotation, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.DNSAnnotation))
	})
	return ret, err
}

// Get retrieves the DNSAnnotation from the indexer for a given namespace and name.
func (s dNSAnnotationNamespaceLister) Get(name string) (*v1alpha1.DNSAnnotation, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha1.Resource("dnsannotation"), name)
	}
	return obj.(*v1alpha1.DNSAnnotation), nil
}
