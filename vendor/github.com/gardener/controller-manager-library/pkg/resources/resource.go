/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package resources

import (
	"fmt"
	"reflect"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/gardener/controller-manager-library/pkg/logger"

	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
)

type _resource struct {
	AbstractResource
	info   *Info
	client restclient.Interface
}

var _ Interface = &_resource{}

type namespacedResource struct {
	resource  *AbstractResource
	namespace string
	lister    NamespacedLister
}

/////////////////////////////////////////////////////////////////////////////////

func newResource(ctx ResourceContext, otype, ltype reflect.Type, gvk schema.GroupVersionKind) (*_resource, error) {
	info, err := ctx.Get(gvk)
	if err != nil {
		return nil, err
	}

	client, err := ctx.GetClient(gvk.GroupVersion())
	if err != nil {
		return nil, err
	}

	if otype == nil {
		otype = unstructuredType
	}
	r := &_resource{
		info:   info,
		client: client,
	}
	r.AbstractResource, _ = NewAbstractResource(ctx, &_i_resource{_resource: r}, otype, ltype, gvk)
	return r, nil
}

func (this *_resource) GetCluster() Cluster {
	return this.ResourceContext().GetCluster()
}

func (this *_resource) ResourceContext() ResourceContext {
	return this.AbstractResource.ResourceContext().(ResourceContext)
}

func (this *_resource) Resources() Resources {
	return this.ResourceContext().Resources()
}

var unstructuredType = reflect.TypeOf(unstructured.Unstructured{})

var unstructuredListType = reflect.TypeOf(unstructured.UnstructuredList{})

func (this *_resource) IsUnstructured() bool {
	return this.ObjectType() == unstructuredType
}

func (this *_resource) Info() *Info {
	return this.info
}

func (this *_resource) Client() restclient.Interface {
	return this.client
}

func (this *_resource) GetParameterCodec() runtime.ParameterCodec {
	return this.ResourceContext().GetParameterCodec()
}

func (this *_resource) AddRawEventHandler(handlers cache.ResourceEventHandlerFuncs) error {
	return this.AddRawSelectedEventHandler(handlers, "", nil)
}

func (this *_resource) AddRawSelectedEventHandler(handlers cache.ResourceEventHandlerFuncs, namespace string, optionsFunc TweakListOptionsFunc) error {
	withNamespace := "global"
	if namespace != "" {
		withNamespace = fmt.Sprintf("namespace %s", namespace)
	}
	logger.Infof("adding watch for %s (cluster %s, %s)", this.GroupVersionKind(), this.GetCluster().GetId(), withNamespace)
	informer, err := this.helper.Internal.I_getInformer(namespace, optionsFunc)
	if err != nil {
		return err
	}
	informer.AddEventHandler(&handlers)
	return nil
}

func (this *_resource) AddEventHandler(handlers ResourceEventHandlerFuncs) error {
	return this.AddRawEventHandler(*convert(this, &handlers))
}

func (this *_resource) AddSelectedEventHandler(handlers ResourceEventHandlerFuncs, namespace string, optionsFunc TweakListOptionsFunc) error {
	return this.AddRawSelectedEventHandler(*convert(this, &handlers), namespace, optionsFunc)
}

func (this *_resource) NormalEventf(name ObjectDataName, reason, msgfmt string, args ...interface{}) {
	this.Resources().Eventf(this.CreateData(name), v1.EventTypeNormal, reason, msgfmt, args...)
}

func (this *_resource) WarningEventf(name ObjectDataName, reason, msgfmt string, args ...interface{}) {
	this.Resources().Eventf(this.CreateData(name), v1.EventTypeWarning, reason, msgfmt, args...)
}

func (this *_resource) namespacedRequest(req *restclient.Request, namespace string) *restclient.Request {
	return req.NamespaceIfScoped(namespace, this.Namespaced()).Resource(this.Name())
}

func (this *_resource) resourceRequest(req *restclient.Request, obj ObjectDataName, sub ...string) *restclient.Request {
	if this.Namespaced() && obj != nil {
		req = req.Namespace(obj.GetNamespace())
	}
	return req.Resource(this.Name()).SubResource(sub...)
}

func (this *_resource) objectRequest(req *restclient.Request, obj ObjectDataName, sub ...string) *restclient.Request {
	return this.resourceRequest(req, obj, sub...).Name(obj.GetName())
}
