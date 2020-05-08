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
	"os"
	"time"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/Masterminds/semver"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"

	"k8s.io/apimachinery/pkg/runtime/schema"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const DEFAULT = "default"

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
	GetServerVersion() *semver.Version
	GetAttr(key interface{}) interface{}
	SetAttr(key, value interface{})
	GetObject(interface{}) (resources.Object, error)
	GetObjectInto(resources.ObjectName, resources.ObjectData) (resources.Object, error)
	GetCachedObject(interface{}) (resources.Object, error)
	GetResource(groupKind schema.GroupKind) (resources.Interface, error)
	Config() restclient.Config
	Resources() resources.Resources
	ResourceContext() resources.ResourceContext
	Definition() Definition

	WithScheme(scheme *runtime.Scheme) (Interface, error)
	resources.ClusterSource
}

type Extension interface {
	ExtendConfig(def Definition, cfg *Config)
	Extend(cluster Interface, config *Config) error
}

type _Cluster struct {
	name       string
	id         string
	definition Definition
	kubeConfig *restclient.Config
	ctx        context.Context
	logctx     logger.LogContext
	rctx       resources.ResourceContext
	resources  resources.Resources
	attributes map[interface{}]interface{}
}

var _ Interface = &_Cluster{}

func (this *_Cluster) GetCluster() resources.Cluster {
	return this
}

func (this *_Cluster) GetName() string {
	return this.name
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

func (this *_Cluster) Resources() resources.Resources {
	return this.resources
}

func (this *_Cluster) ResourceContext() resources.ResourceContext {
	return this.rctx
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

func (this *_Cluster) GetServerVersion() *semver.Version {
	return this.rctx.GetServerVersion()
}

func (this *_Cluster) setup(logger logger.LogContext) error {
	rctx, err := resources.NewResourceContext(this.ctx, this, this.definition.Scheme(), 0*time.Second)
	if err != nil {
		return err
	}
	this.rctx = rctx
	this.resources = this.rctx.Resources()
	return nil
}

func (this *_Cluster) WithScheme(scheme *runtime.Scheme) (Interface, error) {
	if scheme == nil || this.rctx.Scheme() == scheme {
		return this, nil
	}
	logger.Infof("  clone cluster %q[%s] for new scheme", this.name, this.id)
	return CreateClusterForScheme(this.ctx, this.logctx, this.definition, this.id, this.kubeConfig, scheme)
}

func CreateCluster(ctx context.Context, logger logger.LogContext, def Definition, id string, kubeconfig string) (Interface, error) {
	if kubeconfig == "" {
		kubeconfig = os.Getenv("KUBECONFIG")
	}
	name := def.Name()
	logger.Infof("using %q for cluster %q[%s]", kubeconfig, name, id)
	kubeConfig, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create cluster %q: %s", name, err)
	}
	return CreateClusterForScheme(ctx, logger, def, id, kubeConfig, nil)
}

func CreateClusterForScheme(ctx context.Context, logger logger.LogContext, def Definition, id string, kubeconfig *restclient.Config, scheme *runtime.Scheme) (Interface, error) {
	cluster := &_Cluster{name: def.Name(), attributes: map[interface{}]interface{}{}}

	if def.Scheme() == nil {
		scheme = resources.DefaultScheme()
	}
	if scheme != nil && def.Scheme() != scheme {
		def = def.Configure().Scheme(scheme).Definition()
	}
	cluster.ctx = ctx
	cluster.logctx = logger
	cluster.definition = def
	cluster.id = id
	cluster.kubeConfig = kubeconfig

	err := cluster.setup(logger)
	if err != nil {
		return nil, err
	}

	return cluster, nil
}

///////////////////////////////////////////////////////////////////////////////
// cluster set
///////////////////////////////////////////////////////////////////////////////

type clusters map[string]Interface

func (this clusters) Names() utils.StringSet {
	set := utils.StringSet{}
	for n := range this {
		set.Add(n)
	}
	return set
}

type Clusters interface {
	Names() utils.StringSet
	GetCluster(name string) Interface
	GetById(id string) Interface
	GetClusters(name ...string) (Clusters, error)

	EffectiveNames() utils.StringSet
	GetEffective(name string) Interface
	GetAliases(name string) utils.StringSet

	GetObject(key resources.ClusterObjectKey) (resources.Object, error)
	GetCachedObject(key resources.ClusterObjectKey) (resources.Object, error)

	Ids() utils.StringSet

	String() string

	WithScheme(scheme *runtime.Scheme) (Clusters, error)
	Cache() SchemeCache
}

type _Clusters struct {
	cache     SchemeCache
	infos     map[string]string
	mapped    map[string]utils.StringSet
	clusters  clusters
	effective clusters
	byid      clusters
}

var _ Clusters = &_Clusters{}

func NewClusters(cache SchemeCache) *_Clusters {
	if cache == nil {
		cache = NewSchemeCache()
	}
	return &_Clusters{
		cache,
		map[string]string{},
		map[string]utils.StringSet{},
		clusters{},
		clusters{},
		clusters{},
	}
}

func (this *_Clusters) Cache() SchemeCache {
	return this.cache
}

func (this *_Clusters) Names() utils.StringSet {
	return this.clusters.Names()
}

func (this *_Clusters) EffectiveNames() utils.StringSet {
	return this.effective.Names()
}

func (this *_Clusters) Ids() utils.StringSet {
	return this.byid.Names()
}

func (this *_Clusters) WithScheme(scheme *runtime.Scheme) (Clusters, error) {
	var err error

	if scheme == nil {
		return this, nil
	}
	modified := false
	result := NewClusters(this.cache)
	for n, c := range this.clusters {
		mapped := result.GetEffective(c.GetName())
		if mapped == nil {
			mapped, err = this.cache.WithScheme(c, scheme)
			if err != nil {
				return nil, err
			}
			if mapped != c {
				modified = true
			}
		}
		result.Add(n, mapped, this.infos[n])
	}
	if modified {
		return result, nil
	}
	return this, nil
}

func (this *_Clusters) Add(name string, cluster Interface, info ...interface{}) {
	if len(info) > 0 {
		this.infos[name] = fmt.Sprint(info...)
	} else {
		this.infos[name] = name
	}
	this.cache.Add(cluster)
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
	clusters := NewClusters(this.cache)
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
		return nil, fmt.Errorf("cluster with id %q not found", key.Cluster())
	}
	return cluster.GetObject(key.ObjectKey())
}

func (this *_Clusters) GetCachedObject(key resources.ClusterObjectKey) (resources.Object, error) {
	cluster := this.GetById(key.Cluster())
	if cluster == nil {
		return nil, fmt.Errorf("cluster with id %q not found", key.Cluster())
	}
	return cluster.GetCachedObject(key.ObjectKey())
}
