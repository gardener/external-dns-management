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

// DNSEntryLister helps list DNSEntries.
// All objects returned here must be treated as read-only.
type DNSEntryLister interface {
	// List lists all DNSEntries in the indexer.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha1.DNSEntry, err error)
	// DNSEntries returns an object that can list and get DNSEntries.
	DNSEntries(namespace string) DNSEntryNamespaceLister
	DNSEntryListerExpansion
}

// dNSEntryLister implements the DNSEntryLister interface.
type dNSEntryLister struct {
	indexer cache.Indexer
}

// NewDNSEntryLister returns a new DNSEntryLister.
func NewDNSEntryLister(indexer cache.Indexer) DNSEntryLister {
	return &dNSEntryLister{indexer: indexer}
}

// List lists all DNSEntries in the indexer.
func (s *dNSEntryLister) List(selector labels.Selector) (ret []*v1alpha1.DNSEntry, err error) {
	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.DNSEntry))
	})
	return ret, err
}

// DNSEntries returns an object that can list and get DNSEntries.
func (s *dNSEntryLister) DNSEntries(namespace string) DNSEntryNamespaceLister {
	return dNSEntryNamespaceLister{indexer: s.indexer, namespace: namespace}
}

// DNSEntryNamespaceLister helps list and get DNSEntries.
// All objects returned here must be treated as read-only.
type DNSEntryNamespaceLister interface {
	// List lists all DNSEntries in the indexer for a given namespace.
	// Objects returned here must be treated as read-only.
	List(selector labels.Selector) (ret []*v1alpha1.DNSEntry, err error)
	// Get retrieves the DNSEntry from the indexer for a given namespace and name.
	// Objects returned here must be treated as read-only.
	Get(name string) (*v1alpha1.DNSEntry, error)
	DNSEntryNamespaceListerExpansion
}

// dNSEntryNamespaceLister implements the DNSEntryNamespaceLister
// interface.
type dNSEntryNamespaceLister struct {
	indexer   cache.Indexer
	namespace string
}

// List lists all DNSEntries in the indexer for a given namespace.
func (s dNSEntryNamespaceLister) List(selector labels.Selector) (ret []*v1alpha1.DNSEntry, err error) {
	err = cache.ListAllByNamespace(s.indexer, s.namespace, selector, func(m interface{}) {
		ret = append(ret, m.(*v1alpha1.DNSEntry))
	})
	return ret, err
}

// Get retrieves the DNSEntry from the indexer for a given namespace and name.
func (s dNSEntryNamespaceLister) Get(name string) (*v1alpha1.DNSEntry, error) {
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1alpha1.Resource("dnsentry"), name)
	}
	return obj.(*v1alpha1.DNSEntry), nil
}
