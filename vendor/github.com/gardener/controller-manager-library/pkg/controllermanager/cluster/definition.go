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
)

const CLUSTERID_GROUP = "gardener.cloud"

type Definitions interface {
	Get(name string) Definition
	CreateClusters(ctx context.Context, logger logger.LogContext, cfg *config.Config, names utils.StringSet) (Clusters, error)
	ExtendConfig(cfg *config.Config)
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
	var err error
	var c Interface

	clusters := NewClusters()
	this.lock.RLock()
	defer this.lock.RUnlock()

	default_request := this.definitions[DEFAULT]

	logger.Infof("required clusters: %s", names)

	found := -1
	missing := names
	for len(missing) > 0 && found != len(clusters.clusters) {
		found = len(clusters.clusters)
		names = missing
		missing = utils.StringSet{}
		for name := range names {
			fallback := ""
			req := this.definitions[name]
			if clusters.GetCluster(name) == nil {
				opt := cfg.GetOption(req.ConfigOptionName())
				if name != DEFAULT && opt != nil && opt.StringValue() != "" {
					c, err = this.create(ctx, logger, cfg, req, opt.StringValue())
					if err != nil {
						return nil, err
					}
				} else {
					if req.Fallback() == "" || req.Fallback() == DEFAULT {
						c = clusters.effective[DEFAULT]
						if c == nil {
							opt := cfg.GetOption(default_request.ConfigOptionName())
							configname := ""
							if opt != nil {
								configname = opt.StringValue()
							}
							c, err = this.create(ctx, logger, cfg, default_request, configname)
							if err != nil {
								return nil, err
							}
						}
						fallback = fmt.Sprintf(" using default fallback")
					} else {
						c = clusters.effective[req.Fallback()]
						fallback = fmt.Sprintf(" using explicit fallback %q", req.Fallback())
					}
				}
				if c != nil {
					logger.Infof("adding cluster %q[%s] as %q%s", c.GetName(), c.GetId(), name, fallback)
					clusters.Add(name, c)
				} else {
					missing.Add(name)
				}
			}
		}
	}
	if len(missing) > 0 {
		return nil, fmt.Errorf("unresolved cluster fallbacks %s", missing)
	}

	return clusters, nil
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
