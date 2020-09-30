/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package abstract

import (
	"context"
	"reflect"
	"sync"

	"github.com/gardener/controller-manager-library/pkg/resources/errors"

	"github.com/gardener/controller-manager-library/pkg/ctxutil"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

type Factory interface {
	NewResources(ResourceContext, Factory) Resources
	NewResource(resources Resources, gvk schema.GroupVersionKind, otype, ltype reflect.Type) (Resource, error)
	ResolveGVK(ResourceContext, schema.GroupKind, []schema.GroupVersionKind) (schema.GroupVersionKind, error)
}

type AbstractResourceContext struct {
	context.Context

	lock      sync.Mutex
	self      ResourceContext
	decoder   *Decoder
	factory   Factory
	resources Resources
}

func NewAbstractResourceContext(ctx context.Context, self ResourceContext, scheme *runtime.Scheme, factory Factory) *AbstractResourceContext {
	ctx = ctxutil.CancelContext(ctx)
	if scheme == nil {
		scheme = DefaultScheme()
	}
	return &AbstractResourceContext{
		Context: ctx,
		decoder: NewDecoder(scheme),
		factory: factory,
		self:    self,
	}
}

func (this *AbstractResourceContext) Lock() {
	this.lock.Lock()
}

func (this *AbstractResourceContext) Unlock() {
	this.lock.Unlock()
}

func (this *AbstractResourceContext) Scheme() *runtime.Scheme {
	return this.decoder.scheme
}

func (this *AbstractResourceContext) Decoder() *Decoder {
	return this.decoder
}

func (c *AbstractResourceContext) ObjectKinds(obj runtime.Object) ([]schema.GroupVersionKind, bool, error) {
	return c.decoder.scheme.ObjectKinds(obj)
}

func (c *AbstractResourceContext) KnownTypes(gv schema.GroupVersion) map[string]reflect.Type {
	return c.decoder.scheme.KnownTypes(gv)
}

func (this *AbstractResourceContext) GetGroups() []schema.GroupVersion {
	grps := []schema.GroupVersion{}
	found := map[schema.GroupVersion]struct{}{}

	for gvk := range scheme.AllKnownTypes() {
		gk := gvk.GroupVersion()
		if _, ok := found[gk]; !ok {
			grps = append(grps, gk)
			found[gk] = struct{}{}
		}
	}
	return grps
}

func (this *AbstractResourceContext) GetGVKForGK(gk schema.GroupKind) (schema.GroupVersionKind, error) {
	found := []schema.GroupVersionKind{}

	for gvk := range this.Scheme().AllKnownTypes() {
		if gvk.GroupKind() == gk {
			found = append(found, gvk)
		}
	}
	return this.factory.ResolveGVK(this.self, gk, found)
}

func (this *AbstractResourceContext) GetGVK(obj runtime.Object) (schema.GroupVersionKind, error) {
	var empty schema.GroupVersionKind

	gvks, _, err := this.Scheme().ObjectKinds(obj)
	if err != nil {
		return empty, errors.ErrUnexpectedType.Wrap(err, "resource object", obj)
	}

	found := []schema.GroupVersionKind{}
	for _, gvk := range gvks {
		if len(found) == 0 {
			found = append(found, gvk)
		} else {
			if gvk.GroupKind() == found[0].GroupKind() {
				found = append(found, gvk)
			} else {
				return empty, errors.New(errors.ERR_NON_UNIQUE_MAPPING, "non unique mapping for %s", reflect.TypeOf(obj))
			}
		}
	}
	if len(found) == 0 {
		return empty, errors.ErrUnexpectedType.New("resource object", obj)
	}
	return this.factory.ResolveGVK(this.self, found[0].GroupKind(), gvks)
}

func (this *AbstractResourceContext) Resources() Resources {
	this.lock.Lock()
	defer this.lock.Unlock()

	if this.resources == nil {
		this.resources = this.factory.NewResources(this.self, this.factory)
	}
	return this.resources
}
