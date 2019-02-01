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

package controllermanager

import (
	"context"
	"fmt"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/resources/access"
	"strings"
	"sync"
	"time"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/config"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/ctxutil"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/server"
)

type ControllerManager struct {
	lock sync.Mutex
	logger.LogContext

	name       string
	definition *Definition

	ctx           context.Context
	config        *config.Config
	clusters      cluster.Clusters
	registrations controller.Registrations
	plain_groups  map[string]StartupGroup
	lease_groups  map[string]StartupGroup
	shared        map[interface{}]interface{}
	//shared_options map[string]*config.ArbitraryOption
}

var _ controller.Environment = &ControllerManager{}

type Controller interface {
	GetName() string
	Owning() controller.ResourceKey
	GetDefinition() controller.Definition
	GetClusterHandler(name string) (*controller.ClusterHandler, error)

	Check() error
	Prepare() error
	Run()
}

func NewControllerManager(ctx context.Context, def *Definition) (*ControllerManager, error) {
	config := config.Get(ctx)
	ctx = context.WithValue(ctx, resources.ATTR_EVENTSOURCE, def.GetName())

	if config.NamespaceRestriction {
		access.RegisterNamespaceOnlyAccess()
	}
	groups := def.Groups()

	logger.Infof("configured groups: %s", groups.AllGroups())

	if def.ControllerDefinitions().Size() == 0 {
		return nil, fmt.Errorf("no controller registered")
	}

	logger.Infof("configured controllers: %s", def.ControllerDefinitions().Names())

	active, err := groups.Activate(strings.Split(config.Controllers, ","))
	if err != nil {
		return nil, err
	}

	registrations, err := def.Registrations(active.AsArray()...)
	if err != nil {
		return nil, err
	}
	if len(registrations) == 0 {
		return nil, fmt.Errorf("no controller activated")
	}

	set, err := def.ControllerDefinitions().DetermineRequestedClusters(def.ClusterDefinitions(), registrations.Names())
	if err != nil {
		return nil, err
	}

	lgr := logger.New()
	clusters, err := def.ClusterDefinitions().CreateClusters(ctx, lgr, config, set)
	if err != nil {
		return nil, err
	}

	cm := &ControllerManager{
		LogContext: lgr,
		clusters:   clusters,

		name:          def.GetName(),
		definition:    def,
		config:        config,
		registrations: registrations,

		plain_groups: map[string]StartupGroup{},
		lease_groups: map[string]StartupGroup{},
		shared:       map[interface{}]interface{}{},
	}

	ctx = logger.Set(ctxutil.SyncContext(ctx), lgr)
	ctx = context.WithValue(ctx, cmkey, cm)
	cm.ctx = ctx
	return cm, nil
}

func (c *ControllerManager) GetName() string {
	return c.name
}

func (c *ControllerManager) GetContext() context.Context {
	return c.ctx
}

func (c *ControllerManager) GetConfig() *config.Config {
	return c.config
}

func (c *ControllerManager) GetCluster(name string) cluster.Interface {
	return c.clusters.GetCluster(name)
}

func (c *ControllerManager) GetClusters() cluster.Clusters {
	return c.clusters
}

func (c *ControllerManager) GetSharedValue(key interface{}) interface{} {
	return c.shared[key]
}

func (c *ControllerManager) GetOrCreateSharedValue(key interface{}, create func(*ControllerManager) interface{}) interface{} {
	c.lock.Lock()
	defer c.lock.Unlock()
	v, ok := c.shared[key]
	if !ok {
		v = create(c)
		c.shared[key] = v
	}
	return v
}

func (c *ControllerManager) Run() error {
	c.Infof("run %s\n", c.name)

	if c.config.ServerPortHTTP > 0 {
		server.Serve(c.ctx, "", c.config.ServerPortHTTP)
	}

	for _, def := range c.registrations {
		lines := strings.Split(def.String(), "\n")
		c.Infof("creating %s", lines[0])
		for _, l := range lines[1:] {
			c.Info(l)
		}
		cmp, err := c.definition.GetMappingsFor(def.GetName())
		if err != nil {
			return err
		}
		cntr, err := controller.NewController(c, def, cmp)
		if err != nil {
			return err
		}

		if def.RequireLease() {
			c.getLeaseStartupGroup(cntr.GetMainCluster()).Add(cntr)
		} else {
			c.getPlainStartupGroup(cntr.GetMainCluster()).Add(cntr)
		}
	}

	err := c.startGroups(c.plain_groups, c.lease_groups)
	if err != nil {
		return err
	}

	<-c.ctx.Done()
	c.Info("waiting for controllers to shutdown")
	ctxutil.SyncPointWait(c.ctx, 120*time.Second)
	c.Info("exit controller manager")
	return nil
}

// checkController does all the checks that might cause startController to fail
// after the check startController can execute without error
func (c *ControllerManager) checkController(cntr Controller) error {
	return cntr.Check()
}

// startController finally starts the controller
// all error conditions MUST also be checked
// in checkController, so after a successful checkController
// startController MUST not return an error.
func (c *ControllerManager) startController(cntr Controller) error {
	err := cntr.Prepare()
	if err != nil {
		return err
	}

	ctxutil.SyncPointRunAndCancelOnExit(c.ctx, cntr.Run)
	return nil
}
