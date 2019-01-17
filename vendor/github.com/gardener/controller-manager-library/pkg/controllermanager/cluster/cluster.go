/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 *
 */

package cluster

import (
	"context"
	"fmt"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/config"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"os"
	"time"

	// "github.com/gardener/controller-manager-library/pkg/client/gardenextensions/clientset/versioned/scheme"
	"github.com/gardener/controller-manager-library/pkg/clientsets"
	"github.com/gardener/controller-manager-library/pkg/informerfactories"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"

	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const DEFAULT = "default"

const ID_SUB_OPTION = ".id"

func Canonical(names []string) []string {
	if names == nil {
		return []string{DEFAULT}
	}
	r := []string{}
	s := utils.StringSet{}
	for _, n := range names {
		if !s.Contains(n) {
			r = append(r, n)
			s.Add(n)
		}
	}
	return r
}

///////////////////////////////////////////////////////////////////////////////
// cluster
///////////////////////////////////////////////////////////////////////////////

type Interface interface {
	GetName() string
	GetId() string
	GetAttr(key interface{}) interface{}
	SetAttr(key, value interface{})
	Clientsets() clientsets.Interface
	InformerFactories() informerfactories.Interface
	GetObject(interface{}) (resources.Object, error)
	GetObjectInto(resources.ObjectName, resources.ObjectData) (resources.Object, error)
	GetCachedObject(interface{}) (resources.Object, error)
	GetResource(groupKind schema.GroupKind) (resources.Interface, error)
	Config() restclient.Config
	Resources() resources.Resources
	IsLocal() bool
	Definition() Definition
}

type Extension interface {
	ExtendConfig(def Definition, cfg *config.Config)
	Extend(cluster Interface, config *config.Config) error
}

type _Cluster struct {
	name                    string
	id                      string
	definition              Definition
	local                   bool
	kubeConfig              *restclient.Config
	clientsets              clientsets.Interface
	sharedInformerFactories informerfactories.Interface
	ctx                     context.Context
	rctx                    resources.ResourceContext
	resources               resources.Resources
	attributes              map[interface{}]interface{}
}

var _ Interface = &_Cluster{}

func (this *_Cluster) GetName() string {
	return this.name
}

func (this *_Cluster) IsLocal() bool {
	return this.local
}

func (this *_Cluster) Definition() Definition {
	return this.definition
}

func (this *_Cluster) GetId() string {
	if this.id != "" {
		return this.id
	}
	return this.name + "/" + CLUSTERID_GROUP
}

func (this *_Cluster) SetId(id string) {
	this.id = id
}

func (this *_Cluster) String() string {
	return this.name
}

func (this *_Cluster) GetAttr(key interface{}) interface{} {
	return this.attributes[key]
}

func (this *_Cluster) SetAttr(key, value interface{}) {
	this.attributes[key] = value
}

func (this *_Cluster) Config() restclient.Config {
	return *this.kubeConfig
}

func (this *_Cluster) Clientsets() clientsets.Interface {
	return this.clientsets
}

func (this *_Cluster) Resources() resources.Resources {
	return this.resources
}

func (this *_Cluster) InformerFactories() informerfactories.Interface {
	return this.sharedInformerFactories
}

func (this *_Cluster) GetObject(spec interface{}) (resources.Object, error) {
	return this.resources.GetObject(spec)
}

func (this *_Cluster) GetObjectInto(name resources.ObjectName, data resources.ObjectData) (resources.Object, error) {
	return this.resources.GetObjectInto(name, data)
}

func (this *_Cluster) GetCachedObject(spec interface{}) (resources.Object, error) {
	return this.resources.GetCachedObject(spec)
}

func (this *_Cluster) GetResource(groupKind schema.GroupKind) (resources.Interface, error) {
	return this.resources.Get(groupKind)
}

func (this *_Cluster) setup(logger logger.LogContext) error {
	rctx, err := resources.NewResourceContext(this.ctx, this, nil, 0*time.Second)
	if err != nil {
		return err
	}
	this.rctx = rctx
	this.sharedInformerFactories = informerfactories.NewForClientsets(this.clientsets)
	this.resources = this.rctx.Resources()
	return nil
}

func CreateCluster(ctx context.Context, logger logger.LogContext, req Definition, id string, kubeconfig string) (Interface, error) {
	name := req.Name()
	cluster := &_Cluster{name: name, attributes: map[interface{}]interface{}{}}

	if kubeconfig == "" {
		kubeconfig = os.Getenv("KUBECONFIG")
	}

	logger.Infof("using %q for cluster %q[%s]", kubeconfig, name, id)
	kubeConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create cluster %p: %s", name, err)
	}

	cluster.ctx = ctx
	cluster.definition = req
	cluster.id = id
	cluster.kubeConfig = kubeConfig
	cluster.local = kubeconfig == ""
	cluster.clientsets = clientsets.NewForConfig(kubeConfig)

	err = cluster.setup(logger)
	if err != nil {
		return nil, err
	}

	return cluster, nil
}

///////////////////////////////////////////////////////////////////////////////
// cluster set
///////////////////////////////////////////////////////////////////////////////

type Clusters interface {
	GetCluster(name string) Interface
	GetById(id string) Interface
	GetClusters(name ...string) (Clusters, error)

	GetEffective(name string) Interface
	GetAliases(name string) utils.StringSet

	GetObject(key resources.ClusterObjectKey) (resources.Object, error)
	GetCachedObject(key resources.ClusterObjectKey) (resources.Object, error)

	String() string
}

type _Clusters struct {
	infos     map[string]string
	mapped    map[string]utils.StringSet
	clusters  map[string]Interface
	effective map[string]Interface
	byid      map[string]Interface
}

var _ Clusters = &_Clusters{}

func NewClusters() *_Clusters {
	return &_Clusters{
		map[string]string{},
		map[string]utils.StringSet{},
		map[string]Interface{},
		map[string]Interface{},
		map[string]Interface{},
	}
}

func (this *_Clusters) Add(name string, cluster Interface, info ...interface{}) {
	if len(info) > 0 {
		this.infos[name] = fmt.Sprint(info...)
	} else {
		this.infos[name] = name
	}
	this.clusters[name] = cluster
	this.effective[cluster.GetName()] = cluster
	set := this.mapped[cluster.GetName()]
	if set == nil {
		set = utils.StringSet{}
		this.mapped[cluster.GetName()] = set
	}
	this.byid[cluster.GetId()] = cluster
	set.Add(name)
}

func (this *_Clusters) GetEffective(name string) Interface {
	return this.effective[name]
}

func (this *_Clusters) GetCluster(name string) Interface {
	return this.clusters[name]
}

func (this *_Clusters) GetById(id string) Interface {
	return this.byid[id]
}

func (this *_Clusters) GetClusters(name ...string) (Clusters, error) {
	clusters := NewClusters()
	for _, n := range name {
		cluster := this.clusters[n]
		if cluster == nil {
			return nil, fmt.Errorf("unknown cluster %q", n)
		}
		clusters.Add(n, cluster, this.infos[n])
	}
	return clusters, nil
}

func (this *_Clusters) GetAliases(name string) utils.StringSet {
	set := this.mapped[name]
	if set != nil {
		return set.Copy()
	}
	return nil
}

func (this *_Clusters) String() string {
	s := "{"
	sep := ""
	for n, c := range this.clusters {
		if c.GetId() == c.GetName() {
			s = fmt.Sprintf("%s%s %s: %s -> %s", s, sep, n, this.infos[n], c.GetName())
		} else {
			s = fmt.Sprintf("%s%s %s: %s -> %s[%s]", s, sep, n, this.infos[n], c.GetName(), c.GetId())
		}
		sep = ","
	}
	return s + "}"
}

func (this *_Clusters) GetObject(key resources.ClusterObjectKey) (resources.Object, error) {
	cluster := this.GetById(key.Cluster())
	if cluster == nil {
		return nil, fmt.Errorf("cluster with id %q not found")
	}
	return cluster.GetObject(key.ObjectKey)
}

func (this *_Clusters) GetCachedObject(key resources.ClusterObjectKey) (resources.Object, error) {
	cluster := this.GetById(key.Cluster())
	if cluster == nil {
		return nil, fmt.Errorf("cluster with id %q not found")
	}
	return cluster.GetCachedObject(key.ObjectKey)
}
