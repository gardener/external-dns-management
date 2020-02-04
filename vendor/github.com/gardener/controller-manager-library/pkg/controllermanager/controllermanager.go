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
	"github.com/gardener/controller-manager-library/pkg/utils"
	"io/ioutil"
	"log"
	"os"
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
	controller.SharedAttributes

	name       string
	definition *Definition

	ctx           context.Context
	config        *config.Config
	clusters      cluster.Clusters
	registrations controller.Registrations
	plain_groups  map[string]StartupGroup
	lease_groups  map[string]StartupGroup
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

	for n := range def.controller_defs.Names() {
		for _, r := range def.controller_defs.Get(n).RequiredControllers() {
			if def.controller_defs.Get(r) == nil {
				return nil, fmt.Errorf("controller %q requires controller %q, which is not declared", n, r)
			}
		}
	}

	if config.NamespaceRestriction && config.DisableNamespaceRestriction {
		log.Fatalf("contradiction options given for namespace restriction")
	}
	if !config.DisableNamespaceRestriction {
		config.NamespaceRestriction = true
	}
	config.DisableNamespaceRestriction = false

	if config.NamespaceRestriction {
		logger.Infof("enable namespace restriction for access control")
		access.RegisterNamespaceOnlyAccess()
	} else {
		logger.Infof("disable namespace restriction for access control")
	}
	if config.Namespace == "" {
		n := os.Getenv("NAMESPACE")
		if n != "" {
			config.Namespace = n
		} else {
			f := "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
			bytes, err := ioutil.ReadFile(f)
			if err == nil {
				n = string(bytes)
				n = strings.TrimSpace(n)
				if n != "" {
					config.Namespace = n

				}
			}
		}
	}

	name := def.GetName()
	if config.Name != "" {
		name = config.Name
	}

	if config.Namespace == "" {
		config.Namespace = "kube-system"
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

	added := utils.StringSet{}
	for c := range active {
		req, err := def.controller_defs.GetRequiredControllers(c)
		if err != nil {
			return nil, err
		}
		added.AddSet(req)
	}
	added, _ = active.DiffFrom(added)
	if len(added) > 0 {
		logger.Infof("controllers implied by activated controllers: %s", added)
		active.AddSet(added)
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
		SharedAttributes: controller.SharedAttributes{
			LogContext: lgr,
		},
		clusters: clusters,

		name:          name,
		definition:    def,
		config:        config,
		registrations: registrations,

		plain_groups: map[string]StartupGroup{},
		lease_groups: map[string]StartupGroup{},
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
