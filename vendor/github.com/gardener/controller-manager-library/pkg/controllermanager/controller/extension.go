/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gardener/controller-manager-library/pkg/config"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/sync"

	parentcfg "github.com/gardener/controller-manager-library/pkg/controllermanager/config"
	areacfg "github.com/gardener/controller-manager-library/pkg/controllermanager/controller/config"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/extension"
	"github.com/gardener/controller-manager-library/pkg/ctxutil"
	"github.com/gardener/controller-manager-library/pkg/utils"
)

const TYPE = areacfg.OPTION_SOURCE

func init() {
	extension.RegisterExtension(&ExtensionType{DefaultRegistry()})
}

type ExtensionType struct {
	Registry
}

var _ extension.ExtensionType = &ExtensionType{}

func NewExtensionType() *ExtensionType {
	return &ExtensionType{NewRegistry()}
}

func (this *ExtensionType) Name() string {
	return TYPE
}

func (this *ExtensionType) Definition() extension.Definition {
	return NewExtensionDefinition(this.GetDefinitions())
}

////////////////////////////////////////////////////////////////////////////////

type ExtensionDefinition struct {
	extension.ExtensionDefinitionBase
	definitions Definitions
}

func NewExtensionDefinition(defs Definitions) *ExtensionDefinition {
	return &ExtensionDefinition{
		ExtensionDefinitionBase: extension.NewExtensionDefinitionBase(TYPE, []string{"webhooks"}),
		definitions:             defs,
	}
}

func (this *ExtensionDefinition) Description() string {
	return "kubernetes controllers and operators"
}

func (this *ExtensionDefinition) Size() int {
	return this.definitions.Size()
}

func (this *ExtensionDefinition) Names() utils.StringSet {
	return this.definitions.Names()
}

func (this *ExtensionDefinition) Validate() error {
	for n := range this.definitions.Names() {
		for _, r := range this.definitions.Get(n).RequiredControllers() {
			if this.definitions.Get(r) == nil {
				return fmt.Errorf("controller %q requires controller %q, which is not declared", n, r)
			}
		}
	}
	return nil
}

func (this *ExtensionDefinition) ExtendConfig(cfg *parentcfg.Config) {
	my := areacfg.NewConfig()
	this.definitions.ExtendConfig(my)
	cfg.AddSource(areacfg.OPTION_SOURCE, my)
}

func (this *ExtensionDefinition) CreateExtension(cm extension.ControllerManager) (extension.Extension, error) {
	return NewExtension(this.definitions, cm)
}

////////////////////////////////////////////////////////////////////////////////

type clusterMapping struct {
	cluster.Clusters
}

func (this clusterMapping) Map(name string) string {
	c := this.GetCluster(name)
	if c == nil {
		return ""
	}
	return c.GetName()
}

////////////////////////////////////////////////////////////////////////////////

type Extension struct {
	extension.Environment
	sharedAttributes

	config        *areacfg.Config
	definitions   Definitions
	registrations Registrations

	controllers controllers
	after       map[string][]string

	plain_groups map[string]StartupGroup
	lease_groups map[string]StartupGroup
	prepared     map[string]*sync.SyncPoint

	clusters  utils.StringSet
	crossrefs CrossClusterRefs
}

var _ Environment = &Extension{}

type prepare interface {
	Prepare() error
}

func NewExtension(defs Definitions, cm extension.ControllerManager) (*Extension, error) {
	ctx := ctxutil.WaitGroupContext(cm.GetContext(), "controller extension")
	ext := extension.NewDefaultEnvironment(ctx, TYPE, cm)

	cfg := areacfg.GetConfig(cm.GetConfig())

	if cfg.LeaseName == "" {
		cfg.LeaseName = cm.GetName() + "-controllers"
	}
	groups := defs.Groups()
	ext.Infof("configured groups: %s", groups.AllGroups())

	active, err := groups.Members(ext, strings.Split(cfg.Controllers, ","))
	if err != nil {
		return nil, err
	}

	added := utils.StringSet{}
	for c := range active {
		req, err := defs.GetRequiredControllers(c)
		if err != nil {
			return nil, err
		}
		added.AddSet(req)
	}
	added, _ = active.DiffFrom(added)
	if len(added) > 0 {
		ext.Infof("controllers implied by activated controllers: %s", added)
		active.AddSet(added)
		ext.Infof("finally active controllers: %s", active)
	} else {
		ext.Infof("no controllers implied")
	}

	registrations, err := defs.Registrations(active.AsArray()...)
	if err != nil {
		return nil, err
	}

	for n := range registrations {
		var cerr error
		options := cfg.GetSource(n).(*ControllerConfig)
		options.PrefixedShared().VisitSources(func(n string, s config.OptionSource) bool {
			if p, ok := s.(prepare); ok {
				err := p.Prepare()
				if err != nil {
					cerr = err
					return false
				}
			}
			return true
		})
		if cerr != nil {
			return nil, fmt.Errorf("invalid config for controller %q: %s", n, cerr)
		}
	}

	_, after, err := extension.Order(registrations)
	if err != nil {
		return nil, err
	}
	this := &Extension{
		Environment: ext,
		sharedAttributes: sharedAttributes{
			LogContext: ext,
		},
		config:        cfg,
		definitions:   defs,
		registrations: registrations,
		prepared:      map[string]*sync.SyncPoint{},

		after:        after,
		plain_groups: map[string]StartupGroup{},
		lease_groups: map[string]StartupGroup{},
	}
	this.clusters, this.crossrefs, err = this.definitions.DetermineRequestedClusters(this.ClusterDefinitions(), this.registrations.Names())
	if err != nil {
		return nil, err
	}
	return this, nil
}

func (this *Extension) RequiredClusters() (utils.StringSet, error) {
	return this.clusters, nil
}

func (this *Extension) RequiredClusterIds(clusters cluster.Clusters) utils.StringSet {
	refs := this.crossrefs.Map(clusterMapping{clusters})
	this.Infof("extension %s requires cross cluster references for configured clusters: %s", this.Name(), refs)
	return refs.Targets()
}

func (this *Extension) GetConfig() *areacfg.Config {
	return this.config
}

func (this *Extension) Setup(ctx context.Context) error {
	return nil
}

func (this *Extension) Start(ctx context.Context) error {
	var err error

	if this.ControllerManager().GetClusterIdMigration() != nil {
		mig := this.ControllerManager().GetClusterIdMigration().String()
		if mig != "" {
			this.Infof("found migrations: %s", this.ControllerManager().GetClusterIdMigration())
		}
	}
	for _, def := range this.registrations {
		lines := strings.Split(def.String(), "\n")
		this.Infof("creating %s", lines[0])
		for _, l := range lines[1:] {
			this.Info(l)
		}
		cmp, err := this.definitions.GetMappingsFor(def.Name())
		if err != nil {
			return err
		}
		cntr, err := NewController(this, def, cmp)
		if err != nil {
			return err
		}

		this.controllers = append(this.controllers, cntr)
		this.prepared[cntr.GetName()] = &sync.SyncPoint{}
	}

	this.controllers, err = this.controllers.getOrder(this)
	if err != nil {
		return err
	}

	for _, cntr := range this.controllers {
		def := this.registrations[cntr.GetName()]
		if def.RequireLease() {
			cluster := cntr.GetCluster(def.LeaseClusterName())
			this.getLeaseStartupGroup(cluster).Add(cntr)
		} else {
			this.getPlainStartupGroup(cntr.GetMainCluster()).Add(cntr)
		}

		err := this.checkController(cntr)
		if err != nil {
			return err
		}
	}

	err = this.startGroups(this.plain_groups, this.lease_groups)
	if err != nil {
		return err
	}

	ctxutil.WaitGroupRun(ctx, func() {
		<-this.GetContext().Done()
		this.Info("waiting for controllers to shutdown")
		ctxutil.WaitGroupWait(this.GetContext(), 120*time.Second)
		this.Info("all controllers down now")
	})

	return nil
}

// checkController does all the checks that might cause startController to fail
// after the check startController can execute without error
func (this *Extension) checkController(cntr *controller) error {
	return cntr.check()
}

// startController finally starts the controller
// all error conditions MUST also be checked
// in checkController, so after a successful checkController
// startController MUST not return an error.
func (this *Extension) startController(cntr *controller) error {
	for i, a := range this.after[cntr.GetName()] {
		if i == 0 {
			cntr.Infof("observing initialization requirements: %s", utils.Strings(this.after[cntr.GetName()]...))
		}
		after := this.prepared[a]
		if after != nil {
			if !after.IsReached() {
				cntr.Infof("  startup of %q waiting for %q", cntr.GetName(), a)
				if !after.Sync(this.GetContext()) {
					return fmt.Errorf("setup aborted")
				}
				cntr.Infof("  controller %q is initialized now", a)
			} else {
				cntr.Infof("  controller %q is already initialized", a)
			}
		} else {
			cntr.Infof("  omittimg unused controller %q", a)
		}
	}
	cntr.Infof("starting controller")
	err := cntr.prepare()
	if err != nil {
		return err
	}
	this.prepared[cntr.GetName()].Reach()

	ctxutil.WaitGroupRunAndCancelOnExit(this.GetContext(), cntr.Run)
	return nil
}

////////////////////////////////////////////////////////////////////////////////

func (this *Extension) Enqueue(obj resources.Object) {
	for _, c := range this.controllers {
		c.Enqueue(obj)
	}
}

func (this *Extension) EnqueueKey(key resources.ClusterObjectKey) {
	for _, c := range this.controllers {
		c.EnqueueKey(key)
	}
}
