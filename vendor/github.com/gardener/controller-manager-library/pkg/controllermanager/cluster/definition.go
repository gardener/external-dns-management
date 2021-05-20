/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package cluster

import (
	"context"
	"fmt"
	"sync"

	areacfg "github.com/gardener/controller-manager-library/pkg/controllermanager/config"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

const CLUSTERID_GROUP = "gardener.cloud"

type Definitions interface {
	Get(name string) Definition
	CreateClusters(ctx context.Context, logger logger.LogContext, cfg *areacfg.Config, cache SchemeCache, names utils.StringSet) (Clusters, error)
	ExtendConfig(cfg *areacfg.Config)
	GetScheme() *runtime.Scheme
	ClusterNames() []string
	Reconfigure(modifiers ...ConfigurationModifier) Definitions
}

type ConfigurationModifier func(c Configuration) Configuration

var _ Definitions = &_Definitions{}

type Definition interface {
	Name() string
	Description() string
	ConfigOptionName() string
	Fallback() string
	Scheme() *runtime.Scheme

	Definition() Definition
	Configure() Configuration

	IsMinimalWatchEnforced(schema.GroupKind) bool
	MinimalWatches() []schema.GroupKind
}

type _Definition struct {
	name             string
	fallback         string
	configOptionName string
	description      string
	scheme           *runtime.Scheme
	minimalWatches   resources.GroupKindSet
}

func copy(d Definition) *_Definition {
	return &_Definition{
		d.Name(),
		d.Fallback(),
		d.ConfigOptionName(),
		d.Description(),
		d.Scheme(),
		resources.NewGroupKindSetByArray(d.MinimalWatches()),
	}
}

func (this *_Definition) Name() string {
	return this.name
}
func (this *_Definition) ConfigOptionName() string {
	return this.configOptionName
}
func (this *_Definition) Description() string {
	return this.description
}
func (this *_Definition) Fallback() string {
	return this.fallback
}
func (this *_Definition) Scheme() *runtime.Scheme {
	return this.scheme
}
func (this *_Definition) Definition() Definition {
	copy := this.copy()
	return &copy
}

func (this *_Definition) IsMinimalWatchEnforced(gk schema.GroupKind) bool {
	return this.minimalWatches.Contains(gk)
}

func (this *_Definition) MinimalWatches() []schema.GroupKind {
	return this.minimalWatches.AsArray()
}

func (this *_Definition) Configure() Configuration {
	return Configuration{this.copy()}
}

func (this *_Definition) copy() _Definition {
	copy := *this
	copy.minimalWatches = resources.NewGroupKindSetByArray(this.MinimalWatches())
	return copy
}

////////////////////////////////////////////////////////////////////////////////

type _Definitions struct {
	lock        sync.RWMutex
	definitions Registrations
	scheme      *runtime.Scheme
}

var _ Definition = &_Definition{}
var _ Definitions = &_Definitions{}

func (this *_Definitions) create(ctx context.Context, logger logger.LogContext, cfg *Config, req Definition) (Interface, error) {

	id := cfg.ClusterId
	if id != "" {
		logger.Infof("found id %q for cluster %q", id, req.Name())
	}
	if req.Scheme() == nil {
		req = req.Configure().Scheme(this.scheme).Definition()
	}
	cluster, err := CreateCluster(ctx, logger, req, id, cfg)
	if err != nil {
		return nil, err
	}

	crds := cfg.OmitCRDs
	if crds {
		cluster.SetAttr(SUBOPTION_DISABLE_DEPLOY_CRDS, true)
	}

	if !cfg.MigrationIds.IsEmpty() {
		cluster.AddMigrationIds(cfg.MigrationIds.AsArray()...)
	}
	err = callExtensions(func(e Extension) error { return e.Extend(cluster, cfg) })
	if err != nil {
		return nil, err
	}

	return cluster, nil
}

func (this *_Definitions) CreateClusters(ctx context.Context, logger logger.LogContext, cfg *areacfg.Config, cache SchemeCache, names utils.StringSet) (Clusters, error) {
	clusters := NewClusters(cache)
	this.lock.RLock()
	defer this.lock.RUnlock()

	logger.Infof("required clusters: %s", names)

	lastFound := -1
	missing := names
	for len(missing) > 0 && lastFound != len(clusters.clusters) {
		lastFound = len(clusters.clusters)
		names = missing
		missing = utils.StringSet{}
		for name := range names {
			if clusters.GetCluster(name) == nil {
				err := this.handleCluster(ctx, logger, cfg, clusters, missing, name)
				if err != nil {
					return nil, err
				}
			}
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("unresolved cluster fallbacks %s", missing)
	}

	return clusters, nil
}

func (this *_Definitions) handleCluster(ctx context.Context, logger logger.LogContext, cfg *areacfg.Config, found *_Clusters, missing utils.StringSet, name string) error {
	var err error
	var c Interface
	fallback := ""
	defaultRequest := this.definitions[DEFAULT]
	defaultOptions := cfg.GetSource(configTargetKey(defaultRequest)).(*Config)

	req := this.definitions[name]
	if req == nil {
		return fmt.Errorf("no definition for cluster %s", name)
	}
	ccfg := cfg.GetSource(configTargetKey(req)).(*Config)

	if name != DEFAULT && ccfg.IsConfigured() {
		c, err = this.create(ctx, logger, ccfg, req)
		if err != nil {
			return err
		}
	} else {
		if req.Fallback() == "" || req.Fallback() == DEFAULT {
			c = found.effective[DEFAULT]
			if c == nil {
				c, err = this.create(ctx, logger, defaultOptions, defaultRequest)
				if err != nil {
					return err
				}
			}
			fallback = ""
			if name != DEFAULT {
				fallback = fmt.Sprintf(" using default fallback")
			}
		} else {
			c = found.clusters[req.Fallback()]
			fallback = fmt.Sprintf(" using explicit fallback %s", found.infos[req.Fallback()])
		}
	}
	if c != nil {
		logger.Infof("adding cluster %q[%s](%s) as %q%s", c.GetName(), c.GetId(), c.GetServerVersion().Original(), name, fallback)
		found.Add(name, c, fmt.Sprintf("%s%s", name, fallback))
	} else {
		missing.Add(name)
	}

	return nil
}

func (this *_Definitions) ExtendConfig(cfg *areacfg.Config) {
	this.lock.RLock()
	defer this.lock.RUnlock()

	for _, req := range this.definitions {
		clusterCfg := NewConfig(req)
		cfg.AddSource(configTargetKey(req), clusterCfg)
	}
}

func (this *_Definitions) ClusterNames() []string {
	this.lock.RLock()
	defer this.lock.RUnlock()
	names := []string{}
	for k := range this.definitions {
		names = append(names, k)
	}
	return names
}

func (this *_Definitions) Get(name string) Definition {
	this.lock.RLock()
	defer this.lock.RUnlock()
	if c, ok := this.definitions[name]; ok {
		return c
	}
	return nil
}

func (this *_Definitions) GetScheme() *runtime.Scheme {
	this.lock.RLock()
	defer this.lock.RUnlock()
	return this.scheme
}

func (this *_Definitions) Reconfigure(modifiers ...ConfigurationModifier) Definitions {
	this.lock.RLock()
	defer this.lock.RUnlock()

	definitions := &_Definitions{definitions: Registrations{}, scheme: this.scheme}
	for _, name := range this.ClusterNames() {
		configuration := this.Get(name).Configure()
		for _, modifier := range modifiers {
			configuration = modifier(configuration)
		}
		definitions.definitions[name] = configuration.Definition()
	}
	return definitions
}
