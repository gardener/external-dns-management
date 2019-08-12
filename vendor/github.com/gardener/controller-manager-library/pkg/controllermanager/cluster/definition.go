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
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/utils"

	"k8s.io/apimachinery/pkg/runtime"
)

const CLUSTERID_GROUP = "gardener.cloud"

type Definitions interface {
	Get(name string) Definition
	CreateClusters(ctx context.Context, logger logger.LogContext, cfg *config.Config, names utils.StringSet) (Clusters, error)
	ExtendConfig(cfg *config.Config)
	GetScheme() *runtime.Scheme
}

var _ Definitions = &_Definitions{}

type Definition interface {
	Name() string
	Description() string
	ConfigOptionName() string
	Fallback() string
}

type _Definition struct {
	name             string
	fallback         string
	configOptionName string
	description      string
}

func copy(d Definition) *_Definition {
	return &_Definition{d.Name(), d.Fallback(), d.ConfigOptionName(), d.Description()}
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

////////////////////////////////////////////////////////////////////////////////

func (this *_Definitions) create(ctx context.Context, logger logger.LogContext, cfg *config.Config, req Definition, option string) (Interface, error) {
	idopt := cfg.GetOption(req.ConfigOptionName() + ID_SUB_OPTION)
	id := ""
	if idopt != nil && idopt.Changed() {
		id = idopt.StringValue()
		logger.Infof("found id %q for cluster %q", id, req.Name())
	}
	cluster, err := CreateCluster(ctx, logger, req, id, option)
	if err != nil {
		return nil, err
	}

	err = callExtensions(func(e Extension) error { return e.Extend(cluster, cfg) })
	if err != nil {
		return nil, err
	}

	return cluster, nil
}

func (this *_Definitions) CreateClusters(ctx context.Context, logger logger.LogContext, cfg *config.Config, names utils.StringSet) (Clusters, error) {
	clusters := NewClusters()
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

func (this *_Definitions) handleCluster(ctx context.Context, logger logger.LogContext, cfg *config.Config, found *_Clusters, missing utils.StringSet, name string) error {
	var err error
	var c Interface
	fallback := ""
	defaultRequest := this.definitions[DEFAULT]
	req := this.definitions[name]
	if req == nil {
		return fmt.Errorf("no definition for cluster %s", name)
	}

	opt := cfg.GetOption(req.ConfigOptionName())
	//if opt != nil && opt.StringValue() != "" {
	//	logger.Infof("  handle cluster %s (%s=%s", name, opt.Name, opt.StringValue() )
	//
	//} else {
	//	logger.Infof("  handle cluster %s (no config) fallback=%s", name, req.Fallback())
	//}

	if name != DEFAULT && opt != nil && opt.Changed() {
		c, err = this.create(ctx, logger, cfg, req, opt.StringValue())
		if err != nil {
			return err
		}
	} else {
		if req.Fallback() == "" || req.Fallback() == DEFAULT {
			c = found.effective[DEFAULT]
			if c == nil {
				opt := cfg.GetOption(defaultRequest.ConfigOptionName())
				configname := ""
				if opt != nil {
					configname = opt.StringValue()
				}
				c, err = this.create(ctx, logger, cfg, defaultRequest, configname)
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
		logger.Infof("adding cluster %q[%s] as %q%s", c.GetName(), c.GetId(), name, fallback)
		found.Add(name, c, fmt.Sprintf("%s%s", name, fallback))
	} else {
		missing.Add(name)
	}

	return nil
}

func (this *_Definitions) ExtendConfig(cfg *config.Config) {
	this.lock.RLock()
	defer this.lock.RUnlock()

	for _, req := range this.definitions {
		if req.ConfigOptionName() != "" {
			opt, _ := cfg.AddStringOption(req.ConfigOptionName())
			opt.Description = req.Description()

			opt, _ = cfg.AddStringOption(req.ConfigOptionName() + ID_SUB_OPTION)
			opt.Description = fmt.Sprintf("id for cluster %s", req.Name())
		}
		callExtensions(func(e Extension) error { e.ExtendConfig(req, cfg); return nil })
	}
}
