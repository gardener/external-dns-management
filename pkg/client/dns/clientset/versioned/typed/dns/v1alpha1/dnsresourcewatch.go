/*
Copyright (c) 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Code generated by client-gen. DO NOT EDIT.

package v1alpha1

import (
	"time"

	v1alpha1 "github.com/gardener/external-dns-management/pkg/apis/dns/v1alpha1"
	scheme "github.com/gardener/external-dns-management/pkg/client/dns/clientset/versioned/scheme"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	types "k8s.io/apimachinery/pkg/types"
	watch "k8s.io/apimachinery/pkg/watch"
	rest "k8s.io/client-go/rest"
)

// DNSResourceWatchesGetter has a method to return a DNSResourceWatchInterface.
// A group's client should implement this interface.
type DNSResourceWatchesGetter interface {
	DNSResourceWatches(namespace string) DNSResourceWatchInterface
}

// DNSResourceWatchInterface has methods to work with DNSAnnotation resources.
type DNSResourceWatchInterface interface {
	Create(*v1alpha1.DNSAnnotation) (*v1alpha1.DNSAnnotation, error)
	Update(*v1alpha1.DNSAnnotation) (*v1alpha1.DNSAnnotation, error)
	UpdateStatus(*v1alpha1.DNSAnnotation) (*v1alpha1.DNSAnnotation, error)
	Delete(name string, options *v1.DeleteOptions) error
	DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error
	Get(name string, options v1.GetOptions) (*v1alpha1.DNSAnnotation, error)
	List(opts v1.ListOptions) (*v1alpha1.DNSAnnottaionList, error)
	Watch(opts v1.ListOptions) (watch.Interface, error)
	Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.DNSAnnotation, err error)
	DNSResourceWatchExpansion
}

// dNSResourceWatches implements DNSResourceWatchInterface
type dNSResourceWatches struct {
	client rest.Interface
	ns     string
}

// newDNSResourceWatches returns a DNSResourceWatches
func newDNSResourceWatches(c *DnsV1alpha1Client, namespace string) *dNSResourceWatches {
	return &dNSResourceWatches{
		client: c.RESTClient(),
		ns:     namespace,
	}
}

// Get takes name of the dNSResourceWatch, and returns the corresponding dNSResourceWatch object, and an error if there is any.
func (c *dNSResourceWatches) Get(name string, options v1.GetOptions) (result *v1alpha1.DNSAnnotation, err error) {
	result = &v1alpha1.DNSAnnotation{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("dnsresourcewatches").
		Name(name).
		VersionedParams(&options, scheme.ParameterCodec).
		Do().
		Into(result)
	return
}

// List takes label and field selectors, and returns the list of DNSResourceWatches that match those selectors.
func (c *dNSResourceWatches) List(opts v1.ListOptions) (result *v1alpha1.DNSAnnottaionList, err error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	result = &v1alpha1.DNSAnnottaionList{}
	err = c.client.Get().
		Namespace(c.ns).
		Resource("dnsresourcewatches").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Do().
		Into(result)
	return
}

// Watch returns a watch.Interface that watches the requested dNSResourceWatches.
func (c *dNSResourceWatches) Watch(opts v1.ListOptions) (watch.Interface, error) {
	var timeout time.Duration
	if opts.TimeoutSeconds != nil {
		timeout = time.Duration(*opts.TimeoutSeconds) * time.Second
	}
	opts.Watch = true
	return c.client.Get().
		Namespace(c.ns).
		Resource("dnsresourcewatches").
		VersionedParams(&opts, scheme.ParameterCodec).
		Timeout(timeout).
		Watch()
}

// Create takes the representation of a dNSResourceWatch and creates it.  Returns the server's representation of the dNSResourceWatch, and an error, if there is any.
func (c *dNSResourceWatches) Create(dNSResourceWatch *v1alpha1.DNSAnnotation) (result *v1alpha1.DNSAnnotation, err error) {
	result = &v1alpha1.DNSAnnotation{}
	err = c.client.Post().
		Namespace(c.ns).
		Resource("dnsresourcewatches").
		Body(dNSResourceWatch).
		Do().
		Into(result)
	return
}

// Update takes the representation of a dNSResourceWatch and updates it. Returns the server's representation of the dNSResourceWatch, and an error, if there is any.
func (c *dNSResourceWatches) Update(dNSResourceWatch *v1alpha1.DNSAnnotation) (result *v1alpha1.DNSAnnotation, err error) {
	result = &v1alpha1.DNSAnnotation{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("dnsresourcewatches").
		Name(dNSResourceWatch.Name).
		Body(dNSResourceWatch).
		Do().
		Into(result)
	return
}

// UpdateStatus was generated because the type contains a Status member.
// Add a +genclient:noStatus comment above the type to avoid generating UpdateStatus().

func (c *dNSResourceWatches) UpdateStatus(dNSResourceWatch *v1alpha1.DNSAnnotation) (result *v1alpha1.DNSAnnotation, err error) {
	result = &v1alpha1.DNSAnnotation{}
	err = c.client.Put().
		Namespace(c.ns).
		Resource("dnsresourcewatches").
		Name(dNSResourceWatch.Name).
		SubResource("status").
		Body(dNSResourceWatch).
		Do().
		Into(result)
	return
}

// Delete takes name of the dNSResourceWatch and deletes it. Returns an error if one occurs.
func (c *dNSResourceWatches) Delete(name string, options *v1.DeleteOptions) error {
	return c.client.Delete().
		Namespace(c.ns).
		Resource("dnsresourcewatches").
		Name(name).
		Body(options).
		Do().
		Error()
}

// DeleteCollection deletes a collection of objects.
func (c *dNSResourceWatches) DeleteCollection(options *v1.DeleteOptions, listOptions v1.ListOptions) error {
	var timeout time.Duration
	if listOptions.TimeoutSeconds != nil {
		timeout = time.Duration(*listOptions.TimeoutSeconds) * time.Second
	}
	return c.client.Delete().
		Namespace(c.ns).
		Resource("dnsresourcewatches").
		VersionedParams(&listOptions, scheme.ParameterCodec).
		Timeout(timeout).
		Body(options).
		Do().
		Error()
}

// Patch applies the patch and returns the patched dNSResourceWatch.
func (c *dNSResourceWatches) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1alpha1.DNSAnnotation, err error) {
	result = &v1alpha1.DNSAnnotation{}
	err = c.client.Patch(pt).
		Namespace(c.ns).
		Resource("dnsresourcewatches").
		SubResource(subresources...).
		Name(name).
		Body(data).
		Do().
		Into(result)
	return
}