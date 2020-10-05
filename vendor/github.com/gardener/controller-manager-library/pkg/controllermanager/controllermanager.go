/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package controllermanager

import (
	"context"
	"fmt"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/runtime"

	"github.com/gardener/controller-manager-library/pkg/config"
	"github.com/gardener/controller-manager-library/pkg/ctxutil"
	"github.com/gardener/controller-manager-library/pkg/server"

	"github.com/gardener/controller-manager-library/pkg/configmain"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	areacfg "github.com/gardener/controller-manager-library/pkg/controllermanager/config"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/extension"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/resources/access"
	"github.com/gardener/controller-manager-library/pkg/run"
	"github.com/gardener/controller-manager-library/pkg/utils"
)

type ControllerManager struct {
	logger.LogContext
	lock       sync.Mutex
	extensions extension.Extensions
	order      []string

	namespace  string
	definition *Definition

	context  context.Context
	config   *areacfg.Config
	clusters cluster.Clusters

	migrations resources.ClusterIdMigration
}

var _ extension.ControllerManager = &ControllerManager{}

func NewControllerManager(ctx context.Context, def *Definition) (*ControllerManager, error) {
	maincfg := configmain.Get(ctx)
	cfg := areacfg.GetConfig(maincfg)
	lgr := logger.New()
	logger.Info("using option settings:")
	config.Print(logger.Infof, "", cfg.OptionSet)
	logger.Info("-----------------------")
	ctx = logger.Set(ctxutil.WaitGroupContext(ctx, "controllermanager"), lgr)
	ctx = context.WithValue(ctx, resources.ATTR_EVENTSOURCE, def.GetName()) // golint: ignore

	for _, e := range def.extensions {
		err := e.Validate()
		if err != nil {
			return nil, err
		}
	}

	if cfg.NamespaceRestriction {
		logger.Infof("enable namespace restriction for access control")
		access.RegisterNamespaceOnlyAccess()
	} else {
		logger.Infof("disable namespace restriction for access control")
	}

	if cfg.Name == "" {
		cfg.Name = def.GetName()
	}
	if len(cfg.CRDMaintainer.Idents) == 0 {
		cfg.CRDMaintainer.Idents = utils.NewStringSet(cfg.Name)
	}

	namespace := run.GetConfig(maincfg).Namespace
	if namespace == "" {
		namespace = "kube-system"
	}

	found := false
	for _, e := range def.extensions {
		if e.Size() > 0 {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("no controller manager extension registered")
	}

	for _, e := range def.extensions {
		if e.Size() > 0 {
			logger.Infof("configured %s: %s", e.Name(), e.Names())
		}
	}

	order, _, err := extension.Order(def.extensions)
	if err != nil {
		return nil, fmt.Errorf("controller manager extension cycle: %s", err)
	}
	logger.Infof("found configured controller manager extensions:")
	for _, n := range order {
		logger.Infof(" - %s (%d elements): %s", n, def.extensions[n].Size(), def.extensions[n].Description())
	}

	cm := &ControllerManager{
		LogContext: lgr,
		namespace:  namespace,
		definition: def,
		order:      order,
		config:     cfg,
	}
	ctx = ctx_controllermanager.WithValue(ctx, cm)
	cm.context = ctx

	set := utils.StringSet{}

	cm.extensions = extension.Extensions{}
	for _, n := range order {
		d := def.extensions[n]
		e, err := d.CreateExtension(cm)
		if err != nil {
			return nil, err
		}
		if e == nil {
			logger.Infof("skipping unused extension %q", d.Name())
			continue
		}
		cm.extensions[d.Name()] = e
		s, err := e.RequiredClusters()
		if err != nil {
			return nil, err
		}
		set.AddSet(s)
	}

	if len(cm.extensions) == 0 {
		return nil, fmt.Errorf("no controller manager extension activated")
	}

	clusters, err := def.ClusterDefinitions().CreateClusters(ctx, lgr, cfg, cluster.NewSchemeCache(), set)
	if err != nil {
		return nil, err
	}
	reftgtset := utils.StringSet{}
	for _, e := range cm.extensions {
		req := e.RequiredClusterIds(clusters)
		if err != nil {
			return nil, err
		}
		reftgtset.AddSet(req)
	}

	cm.Infof("enforcing explicit cluster ids for %s", reftgtset)
	for reftgt := range reftgtset {
		c := clusters.GetCluster(reftgt)
		if c != nil {
			if err = c.EnforceExplicitClusterIdentity(cm); err != nil {
				return nil, err
			}
		}
	}

	cm.clusters = clusters
	list := []resources.Cluster{}
	for c := range clusters.Names() {
		list = append(list, clusters.GetCluster(c))
	}
	cm.migrations = resources.ClusterIdMigrationFor(list...)

	for _, n := range cm.order {
		e := cm.extensions[n]
		err = e.Setup(cm.context)
		if err != nil {
			return nil, err
		}
	}

	return cm, nil
}

func (this *ControllerManager) GetName() string {
	return this.config.Name
}

func (this *ControllerManager) GetMaintainer() extension.MaintainerInfo {
	return this.config.CRDMaintainer
}

func (this *ControllerManager) GetNamespace() string {
	return this.namespace
}

func (this *ControllerManager) GetContext() context.Context {
	return this.context
}

func (this *ControllerManager) GetConfig() *areacfg.Config {
	return this.config
}

func (this *ControllerManager) GetExtension(name string) extension.Extension {
	return this.extensions[name]
}

func (this *ControllerManager) ClusterDefinitions() cluster.Definitions {
	return this.definition.ClusterDefinitions()
}

func (this *ControllerManager) GetCluster(name string) cluster.Interface {
	return this.clusters.GetCluster(name)
}

func (this *ControllerManager) GetClusters() cluster.Clusters {
	return this.clusters
}

func (this *ControllerManager) GetClusterIdMigration() resources.ClusterIdMigration {
	return this.migrations
}

func (this *ControllerManager) GetDefaultScheme() *runtime.Scheme {
	return this.definition.cluster_defs.GetScheme()
}

func (this *ControllerManager) Run() error {
	var err error
	this.Infof("run %s\n", this.config.Name)

	server.ServeFromMainConfig(this.context, "httpserver")

	for _, n := range this.order {
		err = this.extensions[n].Start(this.context)
		if err != nil {
			return err
		}
	}

	<-this.context.Done()
	this.Info("waiting for extensions to shutdown")
	ctxutil.WaitGroupWait(this.context, 120*time.Second, "extension shutdown")
	this.Info("all extensions down -> exit controller manager")
	return nil
}
