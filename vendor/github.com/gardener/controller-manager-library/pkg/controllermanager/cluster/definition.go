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

	areacfg "github.com/gardener/controller-manager-library/pkg/controllermanager/config"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/utils"

	"k8s.io/apimachinery/pkg/runtime"
)

const CLUSTERID_GROUP = "gardener.cloud"

type Definitions interface {
	Get(name string) Definition
	CreateClusters(ctx context.Context, logger logger.LogContext, cfg *areacfg.Config, cache SchemeCache, names utils.StringSet) (Clusters, error)
	ExtendConfig(cfg *areacfg.Config)
	GetScheme() *runtime.Scheme
}

var _ Definitions = &_Definitions{}

type Definition interface {
	Name() string
	Description() string
	ConfigOptionName() string
	Fallback() string
	Scheme() *runtime.Scheme

	Definition() Definition
	Configure() Configuration
}

type _Definition struct {
	name             string
	fallback         string
	configOptionName string
	description      string
	scheme           *runtime.Scheme
}

func copy(d Definition) *_Definition {
	return &_Definition{
		d.Name(),
		d.Fallback(),
		d.ConfigOptionName(),
		d.Description(),
		d.Scheme(),
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
	return this
}

func (this *_Definition) Configure() Configuration {
	return Configuration{*this}
}

////////////////////////////////////////////////////////////////////////////////

func (this *_Definitions) create(ctx context.Context, logger logger.LogContext, cfg *Config, req Definition) (Interface, error) {

	id := cfg.ClusterId
	if id != "" {
		logger.Infof("found id %q for cluster %q", id, req.Name())
	}
	if req.Scheme() == nil {
		req = req.Configure().Scheme(this.scheme).Definition()
	}
	cluster, err := CreateCluster(ctx, logger, req, id, cfg.KubeConfig)
	if err != nil {
		return nil, err
	}

	crds := cfg.OmitCRDs
	if crds {
		cluster.SetAttr(SUBOPTION_DISABLE_DEPLOY_CRDS, true)
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

	if name != DEFAULT && ccfg.KubeConfig != "" {
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
		callExtensions(func(e Extension) error { e.ExtendConfig(req, clusterCfg); return nil })
		cfg.AddSource(configTargetKey(req), clusterCfg)
	}
}
