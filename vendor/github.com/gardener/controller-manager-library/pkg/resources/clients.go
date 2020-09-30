/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package resources

import (
	"sync"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	restclient "k8s.io/client-go/rest"
)

type Clients struct {
	lock           sync.Mutex
	scheme         *runtime.Scheme
	config         restclient.Config
	codecfactory   serializer.CodecFactory
	parametercodec runtime.ParameterCodec
	clients        map[schema.GroupVersion]restclient.Interface
}

func NewClients(config restclient.Config, scheme *runtime.Scheme) *Clients {
	client := &Clients{
		config:         config,
		scheme:         scheme,
		clients:        map[schema.GroupVersion]restclient.Interface{},
		codecfactory:   serializer.NewCodecFactory(scheme),
		parametercodec: runtime.NewParameterCodec(scheme),
	}
	client.config.NegotiatedSerializer = serializer.WithoutConversionCodecFactory{CodecFactory: client.codecfactory}
	return client
}

func (c *Clients) NewFor(config restclient.Config) *Clients {
	return NewClients(config, c.scheme)
}

func (c *Clients) GetCodecFactory() serializer.CodecFactory {
	return c.codecfactory
}

func (c *Clients) GetParameterCodec() runtime.ParameterCodec {
	return c.parametercodec
}

func (c *Clients) GetClient(gv schema.GroupVersion) (restclient.Interface, error) {
	c.lock.Lock()
	defer c.lock.Unlock()
	var err error
	client := c.clients[gv]
	if client == nil {
		config := c.config
		config.GroupVersion = &gv
		if gv.Group == "" {
			config.APIPath = "/api"

		} else {
			config.APIPath = "/apis"
		}

		if config.UserAgent == "" {
			config.UserAgent = restclient.DefaultKubernetesUserAgent()
		}

		client, err = restclient.RESTClientFor(&config)
		if err != nil {
			return nil, err
		}
		c.clients[gv] = client
	}
	return client, nil
}
