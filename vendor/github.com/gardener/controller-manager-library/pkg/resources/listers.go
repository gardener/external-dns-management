/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package resources

import (
	"fmt"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/tools/cache"
)

type Lister interface {
	List(selector labels.Selector, consumer func(o interface{})) error
	Namespace(namespace string) NamespacedLister
	Get(name string) (ObjectData, error)
}

type lister struct {
	indexer  cache.Indexer
	resource *Info
}

func NewLister(indexer cache.Indexer, resource *Info) Lister {
	return &lister{indexer: indexer, resource: resource}
}

// func (s *lister) List(selector labels.Selector) (ret []ObjectData, err error) {
//	err = cache.ListAll(s.indexer, selector, func(m interface{}) {
//		ret = append(ret, m.(ObjectData))
//	})
//	return ret, err
// }

func (s *lister) List(selector labels.Selector, consumer func(o interface{})) error {
	return cache.ListAll(s.indexer, selector, consumer)
}

func (s *lister) Get(name string) (ObjectData, error) {

	if s.resource.Namespaced() {
		return nil, errors.NewBadRequest(fmt.Sprintf("info %s is namespaced", s.resource.Name()))
	}
	obj, exists, err := s.indexer.GetByKey(name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1.Resource(s.resource.Name()), name)
	}
	return obj.(ObjectData), nil
}

func (s *lister) Namespace(namespace string) NamespacedLister {
	return &namespacedLister{indexer: s.indexer, namespace: namespace, info: s.resource}
}

type NamespacedLister interface {
	List(selector labels.Selector, consumer func(o interface{})) error
	Get(name string) (ObjectData, error)
}

type namespacedLister struct {
	indexer   cache.Indexer
	info      *Info
	namespace string
}

func (s *namespacedLister) List(selector labels.Selector, consumer func(o interface{})) (err error) {
	if !s.info.Namespaced() {
		return errors.NewBadRequest(fmt.Sprintf("info %s is not namespaced", s.info.Name()))
	}
	return cache.ListAllByNamespace(s.indexer, s.namespace, selector, consumer)
}

func (s *namespacedLister) Get(name string) (ObjectData, error) {
	if !s.info.Namespaced() {
		return nil, errors.NewBadRequest(fmt.Sprintf("info %s is not namespaced", s.info.Name()))
	}
	obj, exists, err := s.indexer.GetByKey(s.namespace + "/" + name)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, errors.NewNotFound(v1.Resource(s.info.Name()), name)
	}
	return obj.(ObjectData), nil
}
