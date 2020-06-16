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

package controller

import (
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/gardener/controller-manager-library/pkg/config"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/extension"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/resources/apiextensions"

	"github.com/gardener/controller-manager-library/pkg/utils"
)

////////////////////////////////////////////////////////////////////////////////
// Watch Selection Functions
//

func NamespaceSelection(namespace string) WatchSelectionFunction {
	return func(c Interface) (string, resources.TweakListOptionsFunc) {
		return namespace, nil
	}
}

func NamespaceByOptionSelection(opt string) WatchSelectionFunction {
	return func(c Interface) (string, resources.TweakListOptionsFunc) {
		namespace, err := c.GetStringOption(opt)
		if err != nil {
			panic(fmt.Errorf("option %q not found for namespace selection in controller resource for %s: %s",
				opt, c.GetName(), err))
		}
		return namespace, nil
	}
}

func LocalNamespaceSelection(c Interface) (string, resources.TweakListOptionsFunc) {
	return c.GetEnvironment().Namespace(), nil
}

////////////////////////////////////////////////////////////////////////////////
// Option Source Creators

func OptionSourceCreator(proto config.OptionSource) extension.OptionSourceCreator {
	return extension.OptionSourceCreatorByExample(proto)
}

///////////////////////////////////////////////////////////////////////////////

type syncerdef struct {
	name     string
	cluster  string
	resource ResourceKey
}

func (this *syncerdef) GetName() string {
	return this.name
}
func (this *syncerdef) GetCluster() string {
	return this.cluster
}
func (this *syncerdef) GetResource() ResourceKey {
	return this.resource
}

///////////////////////////////////////////////////////////////////////////////

type pooldef struct {
	name   string
	size   int
	period time.Duration
}

func (this *pooldef) GetName() string {
	return this.name
}
func (this *pooldef) Size() int {
	return this.size
}
func (this *pooldef) Period() time.Duration {
	return this.period
}

///////////////////////////////////////////////////////////////////////////////

type watchdef struct {
	rescdef
	reconciler string
	pool       string
}

type rescdef struct {
	rtype      ResourceKey
	selectFunc WatchSelectionFunction
}

func (this *rescdef) ResourceType() ResourceKey {
	return this.rtype
}
func (this *rescdef) WatchSelectionFunction() WatchSelectionFunction {
	return this.selectFunc
}
func (this *rescdef) String() string {
	if this.selectFunc != nil {
		return this.rtype.String() + " with selector"
	}
	return this.rtype.String()
}

func (this *watchdef) Reconciler() string {
	return this.reconciler
}
func (this *watchdef) PoolName() string {
	return this.pool
}

///////////////////////////////////////////////////////////////////////////////

type cmddef struct {
	key        utils.Matcher
	reconciler string
	pool       string
}

func (this *cmddef) Key() utils.Matcher {
	return this.key
}
func (this *cmddef) Reconciler() string {
	return this.reconciler
}
func (this *cmddef) PoolName() string {
	return this.pool
}

type _Definition struct {
	name                 string
	main                 rescdef
	reconcilers          map[string]ReconcilerType
	syncers              map[string]SyncerDefinition
	watches              Watches
	commands             Commands
	resource_filters     []ResourceFilter
	after                []string
	before               []string
	required_clusters    []string
	required_controllers []string
	require_lease        bool
	pools                map[string]PoolDefinition
	configs              extension.OptionDefinitions
	configsources        extension.OptionSourceDefinitions
	finalizerName        string
	finalizerDomain      string
	crds                 map[string][]*apiextensions.CustomResourceDefinitionVersions
	activateExplicitly   bool
	scheme               *runtime.Scheme
}

var _ Definition = &_Definition{}

func (this *_Definition) String() string {
	s := fmt.Sprintf("controller %q:\n", this.name)
	s += fmt.Sprintf("  main rsc:    %s\n", this.main)
	s += fmt.Sprintf("  clusters:    %s\n", utils.Strings(this.RequiredClusters()...))
	s += fmt.Sprintf("  required:    %s\n", utils.Strings(this.RequiredControllers()...))
	s += fmt.Sprintf("  after:       %s\n", utils.Strings(this.After()...))
	s += fmt.Sprintf("  before:       %s\n", utils.Strings(this.Before()...))
	s += fmt.Sprintf("  reconcilers: %s\n", toString(this.reconcilers))
	s += fmt.Sprintf("  watches:     %s\n", toString(this.watches))
	s += fmt.Sprintf("  commands:    %s\n", toString(this.commands))
	s += fmt.Sprintf("  pools:       %s\n", toString(this.pools))
	s += fmt.Sprintf("  finalizer:   %s\n", this.FinalizerName())
	s += fmt.Sprintf("  explicit :   %t\n", this.activateExplicitly)
	if this.scheme != nil {
		s += fmt.Sprintf("  scheme is set\n")
	}
	return s
}

func (this *_Definition) Name() string {
	return this.name
}
func (this *_Definition) MainResource() ResourceKey {
	return this.main.ResourceType()
}
func (this *_Definition) MainWatchResource() WatchResource {
	return &this.main
}
func (this *_Definition) Watches() Watches {
	return this.watches
}
func (this *_Definition) Commands() Commands {
	return this.commands
}
func (this *_Definition) Scheme() *runtime.Scheme {
	return this.scheme
}
func (this *_Definition) ResourceFilters() []ResourceFilter {
	return this.resource_filters
}
func (this *_Definition) After() []string {
	return this.after
}
func (this *_Definition) Before() []string {
	return this.before
}
func (this *_Definition) RequiredClusters() []string {
	if len(this.required_clusters) > 0 {
		return this.required_clusters
	}
	return []string{cluster.DEFAULT}
}
func (this *_Definition) RequiredControllers() []string {
	return this.required_controllers
}
func (this *_Definition) RequireLease() bool {
	return this.require_lease
}
func (this *_Definition) FinalizerName() string {
	if this.finalizerName == "" {
		return FinalizerName(this.finalizerDomain, this.name)
	}
	return this.finalizerName
}

func (this *_Definition) CustomResourceDefinitions() map[string][]*apiextensions.CustomResourceDefinitionVersions {
	crds := map[string][]*apiextensions.CustomResourceDefinitionVersions{}
	for n, l := range this.crds {
		crds[n] = append([]*apiextensions.CustomResourceDefinitionVersions{}, l...)
	}
	return this.crds
}

func (this *_Definition) Reconcilers() map[string]ReconcilerType {
	types := map[string]ReconcilerType{}
	for n, d := range this.reconcilers {
		types[n] = d
	}
	return types
}
func (this *_Definition) Syncers() map[string]SyncerDefinition {
	syncers := map[string]SyncerDefinition{}
	for n, d := range this.syncers {
		syncers[n] = d
	}
	return syncers
}
func (this *_Definition) Pools() map[string]PoolDefinition {
	pools := map[string]PoolDefinition{}
	for n, d := range this.pools {
		pools[n] = d
	}
	if len(pools) == 0 {
		pools[DEFAULT_POOL] = &pooldef{DEFAULT_POOL, 5, 30 * time.Second}
	}
	return pools
}
func (this *_Definition) ConfigOptions() map[string]OptionDefinition {
	cfgs := map[string]OptionDefinition{}
	for n, d := range this.configs {
		cfgs[n] = d
	}
	return cfgs
}

func (this *_Definition) ConfigOptionSources() extension.OptionSourceDefinitions {
	cfgs := extension.OptionSourceDefinitions{}
	for n, d := range this.configsources {
		cfgs[n] = d
	}
	return cfgs
}

func (this *_Definition) ActivateExplicitly() bool {
	return this.activateExplicitly
}

////////////////////////////////////////////////////////////////////////////////

type ConfigurationModifier func(c Configuration) Configuration

type Configuration struct {
	settings _Definition
	configState
}

type configState struct {
	previous   *configState
	cluster    string
	pool       string
	reconciler string
}

func (this *configState) pushState() {
	save := *this
	this.previous = &save
}

func Configure(name string) Configuration {
	return Configuration{
		settings: _Definition{
			name:          name,
			reconcilers:   map[string]ReconcilerType{},
			syncers:       map[string]SyncerDefinition{},
			pools:         map[string]PoolDefinition{},
			configs:       extension.OptionDefinitions{},
			configsources: extension.OptionSourceDefinitions{},
		},
		configState: configState{
			cluster:    CLUSTER_MAIN,
			pool:       DEFAULT_POOL,
			reconciler: DEFAULT_RECONCILER,
		},
	}
}

func (this Configuration) With(modifier ...ConfigurationModifier) Configuration {
	save := this.configState
	result := this
	for _, m := range modifier {
		result = m(result)
	}
	result.configState = save
	return result
}

func (this Configuration) Restore() Configuration {
	if &this.configState != nil {
		this.configState = *this.configState.previous
	}
	return this
}

func (this Configuration) Name(name string) Configuration {
	this.settings.name = name
	return this
}

func (this Configuration) After(names ...string) Configuration {
	utils.StringArrayAddUnique(&this.settings.after, names...)
	return this
}

func (this Configuration) Before(names ...string) Configuration {
	utils.StringArrayAddUnique(&this.settings.before, names...)
	return this
}

func (this Configuration) Require(names ...string) Configuration {
	utils.StringArrayAddUnique(&this.settings.required_controllers, names...)
	return this
}

func (this Configuration) MainResource(group, kind string, sel ...WatchSelectionFunction) Configuration {
	return this.MainResourceByKey(NewResourceKey(group, kind), sel...)
}
func (this Configuration) MainResourceByGK(gk schema.GroupKind, sel ...WatchSelectionFunction) Configuration {
	return this.MainResourceByKey(NewResourceKeyByGK(gk), sel...)
}
func (this Configuration) MainResourceByKey(key ResourceKey, sel ...WatchSelectionFunction) Configuration {
	this.settings.main.rtype = key
	if len(sel) > 0 {
		this.settings.main.selectFunc = sel[0]
	}
	return this
}

func (this Configuration) DefaultWorkerPool(size int, period time.Duration) Configuration {
	return this.WorkerPool(DEFAULT_POOL, size, period)
}

func (this Configuration) WorkerPool(name string, size int, period time.Duration) Configuration {
	this.pushState()
	if this.settings.pools[name] != nil {
		panic(fmt.Sprintf("pool %q already defined", name))
	}

	this.settings.pools[name] = &pooldef{name, size, period}
	this.pool = name
	return this
}

func (this Configuration) Pool(name string) Configuration {
	this.pushState()
	this.pool = name
	return this
}

func (this Configuration) DefaultCluster() Configuration {
	return this.Cluster(cluster.DEFAULT)
}

func (this Configuration) Cluster(name string) Configuration {
	this.pushState()
	this.cluster = name
	this.pool = DEFAULT_POOL
	if name != CLUSTER_MAIN {
		for i, n := range this.settings.required_clusters {
			if n == name {
				if i == 0 {
					this.cluster = CLUSTER_MAIN
				}
				return this
			}
		}
		this.settings.required_clusters = append([]string{}, this.settings.required_clusters...)
		this.settings.required_clusters = append(this.settings.required_clusters, name)
	}
	return this
}

func (this Configuration) CustomResourceDefinitions(crds ...apiextensions.CRDSpecification) Configuration {
	m := map[string][]*apiextensions.CustomResourceDefinitionVersions{}
	for k, v := range this.settings.crds {
		m[k] = v
	}
	list := m[this.cluster]
	if list == nil {
		list = []*apiextensions.CustomResourceDefinitionVersions{}
	}
	list = append([]*apiextensions.CustomResourceDefinitionVersions{}, list...)
	for _, crd := range crds {
		vers, err := apiextensions.NewDefaultedCustomResourceDefinitionVersions(crd)
		utils.Must(err)
		list = append(list, vers)
	}
	m[this.cluster] = list
	this.settings.crds = m
	return this
}

func (this Configuration) VersionedCustomResourceDefinitions(crds ...*CustomResourceDefinition) Configuration {
	m := map[string][]*apiextensions.CustomResourceDefinitionVersions{}
	for k, v := range this.settings.crds {
		m[k] = v
	}
	list := m[this.cluster]
	if list == nil {
		list = []*apiextensions.CustomResourceDefinitionVersions{}
	}
	list = append([]*apiextensions.CustomResourceDefinitionVersions{}, list...)

	for _, crd := range crds {
		m[this.cluster] = append(list, crd.GetVersions())
	}
	this.settings.crds = m
	return this
}

func (this Configuration) Syncer(name string, resc ResourceKey) Configuration {
	copy := map[string]SyncerDefinition{}
	for n, s := range this.settings.syncers {
		copy[n] = s
	}
	copy[name] = &syncerdef{name: name, cluster: this.cluster, resource: resc}
	this.settings.syncers = copy
	return this
}

func (this *Configuration) assureWatches() {
	if this.settings.watches == nil {
		this.settings.watches = map[string][]Watch{}
	}
}

func (this Configuration) Watches(keys ...ResourceKey) Configuration {
	return this.ReconcilerWatches(DEFAULT_RECONCILER, keys...)
}
func (this Configuration) WatchesByGK(gks ...schema.GroupKind) Configuration {
	return this.ReconcilerWatchesByGK(DEFAULT_RECONCILER, gks...)
}
func (this Configuration) SelectedWatches(sel WatchSelectionFunction, keys ...ResourceKey) Configuration {
	return this.ReconcilerSelectedWatches(DEFAULT_RECONCILER, sel, keys...)
}
func (this Configuration) SelectedWatchesByGK(sel WatchSelectionFunction, gks ...schema.GroupKind) Configuration {
	return this.ReconcilerSelectedWatchesByGK(DEFAULT_RECONCILER, sel, gks...)
}

func (this Configuration) Watch(group, kind string) Configuration {
	return this.ReconcilerWatches(DEFAULT_RECONCILER, NewResourceKey(group, kind))
}
func (this Configuration) WatchByGK(gk schema.GroupKind) Configuration {
	return this.ReconcilerWatchesByGK(DEFAULT_RECONCILER, gk)
}
func (this Configuration) SelectedWatch(sel WatchSelectionFunction, group, kind string) Configuration {
	return this.ReconcilerSelectedWatches(DEFAULT_RECONCILER, sel, NewResourceKey(group, kind))
}
func (this Configuration) SelectedWatchByGK(sel WatchSelectionFunction, gk schema.GroupKind) Configuration {
	return this.ReconcilerSelectedWatchesByGK(DEFAULT_RECONCILER, sel, gk)
}

func (this Configuration) ForWatches(keys ...ResourceKey) Configuration {
	return this.ReconcilerWatches(this.reconciler, keys...)
}
func (this Configuration) ForWatchesByGK(gks ...schema.GroupKind) Configuration {
	return this.ReconcilerWatchesByGK(this.reconciler, gks...)
}
func (this Configuration) ForSelectedWatches(sel WatchSelectionFunction, keys ...ResourceKey) Configuration {
	return this.ReconcilerSelectedWatches(this.reconciler, sel, keys...)
}
func (this Configuration) ForSelectedWatchesByGK(sel WatchSelectionFunction, gks ...schema.GroupKind) Configuration {
	return this.ReconcilerSelectedWatchesByGK(this.reconciler, sel, gks...)
}

func (this Configuration) ForWatch(group, kind string) Configuration {
	return this.ReconcilerWatches(this.reconciler, NewResourceKey(group, kind))
}
func (this Configuration) ForWatchByGK(gk schema.GroupKind) Configuration {
	return this.ReconcilerWatches(this.reconciler, NewResourceKeyByGK(gk))
}
func (this Configuration) ForSelectedWatch(sel WatchSelectionFunction, group, kind string) Configuration {
	return this.ReconcilerSelectedWatches(this.reconciler, sel, NewResourceKey(group, kind))
}
func (this Configuration) ForSelectedWatchByGK(sel WatchSelectionFunction, gk schema.GroupKind) Configuration {
	return this.ReconcilerSelectedWatchesByGK(this.reconciler, sel, gk)
}

func (this Configuration) ReconcilerWatch(reconciler, group, kind string) Configuration {
	return this.ReconcilerWatches(reconciler, NewResourceKey(group, kind))
}
func (this Configuration) ReconcilerWatchByGK(reconciler string, gk schema.GroupKind) Configuration {
	return this.ReconcilerWatches(reconciler, NewResourceKeyByGK(gk))
}

func (this Configuration) ReconcilerWatches(reconciler string, keys ...ResourceKey) Configuration {
	this.assureWatches()
	for _, key := range keys {
		// logger.Infof("adding watch for %q:%q to pool %q", this.cluster, key, this.pool)
		this.settings.watches[this.cluster] = append(this.settings.watches[this.cluster], &watchdef{rescdef{key, nil}, reconciler, this.pool})
	}
	return this
}
func (this Configuration) ReconcilerWatchesByGK(reconciler string, gks ...schema.GroupKind) Configuration {
	this.assureWatches()
	for _, gk := range gks {
		key := NewResourceKeyByGK(gk)
		// logger.Infof("adding watch for %q:%q to pool %q", this.cluster, key, this.pool)
		this.settings.watches[this.cluster] = append(this.settings.watches[this.cluster], &watchdef{rescdef{key, nil}, reconciler, this.pool})
	}
	return this
}

func (this Configuration) ReconcilerSelectedWatches(reconciler string, sel WatchSelectionFunction, keys ...ResourceKey) Configuration {
	this.assureWatches()
	for _, key := range keys {
		// logger.Infof("adding watch for %q:%q to pool %q", this.cluster, key, this.pool)
		this.settings.watches[this.cluster] = append(this.settings.watches[this.cluster], &watchdef{rescdef{key, sel}, reconciler, this.pool})
	}
	return this
}
func (this Configuration) ReconcilerSelectedWatchesByGK(reconciler string, sel WatchSelectionFunction, gks ...schema.GroupKind) Configuration {
	this.assureWatches()
	for _, gk := range gks {
		key := NewResourceKeyByGK(gk)
		// logger.Infof("adding watch for %q:%q to pool %q", this.cluster, key, this.pool)
		this.settings.watches[this.cluster] = append(this.settings.watches[this.cluster], &watchdef{rescdef{key, sel}, reconciler, this.pool})
	}
	return this
}

func (this Configuration) ActivateExplicitly() Configuration {
	this.settings.activateExplicitly = true
	return this
}

func (this *Configuration) assureCommands() {
	if this.settings.commands == nil {
		this.settings.commands = map[string][]Command{}
	}
}

func (this Configuration) Commands(cmd ...string) Configuration {
	return this.ReconcilerCommands(DEFAULT_RECONCILER, cmd...)
}

func (this Configuration) CommandMatchers(cmd ...utils.Matcher) Configuration {
	return this.ReconcilerCommandMatchers(DEFAULT_RECONCILER, cmd...)
}

func (this Configuration) ForCommands(cmd ...string) Configuration {
	return this.ReconcilerCommands(this.reconciler, cmd...)
}

func (this Configuration) ForCommandMatchers(cmd ...utils.Matcher) Configuration {
	return this.ReconcilerCommandMatchers(this.reconciler, cmd...)
}

func (this Configuration) ReconcilerCommands(reconciler string, cmd ...string) Configuration {
	this.assureCommands()
	for _, cmd := range cmd {
		this.settings.commands[reconciler] = append(this.settings.commands[reconciler], &cmddef{utils.NewStringMatcher(cmd), reconciler, this.pool})
	}
	return this
}
func (this Configuration) ReconcilerCommandMatchers(reconciler string, cmd ...utils.Matcher) Configuration {
	this.assureCommands()
	for _, cmd := range cmd {
		this.settings.commands[reconciler] = append(this.settings.commands[reconciler], &cmddef{cmd, reconciler, this.pool})
	}
	return this
}

func (this Configuration) Reconciler(t ReconcilerType, name ...string) Configuration {
	this.pushState()
	if len(name) == 0 {
		this.settings.reconcilers[DEFAULT_RECONCILER] = t
		this.reconciler = DEFAULT_RECONCILER
	} else {
		for _, n := range name {
			this.settings.reconcilers[n] = t
			this.reconciler = n
		}
	}
	return this
}

func (this Configuration) FinalizerName(name string) Configuration {
	this.settings.finalizerName = name
	return this
}

func (this Configuration) FinalizerDomain(name string) Configuration {
	this.settings.finalizerDomain = name
	return this
}
func (this Configuration) RequireLease() Configuration {
	this.settings.require_lease = true
	return this
}

func (this Configuration) Scheme(scheme *runtime.Scheme) Configuration {
	this.settings.scheme = scheme
	return this
}

func (this Configuration) StringOption(name string, desc string) Configuration {
	return this.addOption(name, config.StringOption, "", desc)
}
func (this Configuration) DefaultedStringOption(name, def string, desc string) Configuration {
	return this.addOption(name, config.StringOption, def, desc)
}

func (this Configuration) StringArrayOption(name string, desc string) Configuration {
	return this.addOption(name, config.StringArrayOption, nil, desc)
}
func (this Configuration) DefaultedStringArrayOption(name string, def []string, desc string) Configuration {
	return this.addOption(name, config.StringArrayOption, def, desc)
}

func (this Configuration) IntOption(name string, desc string) Configuration {
	return this.addOption(name, config.IntOption, 0, desc)
}
func (this Configuration) DefaultedIntOption(name string, def int, desc string) Configuration {
	return this.addOption(name, config.IntOption, def, desc)
}

func (this Configuration) BoolOption(name string, desc string) Configuration {
	return this.addOption(name, config.BoolOption, false, desc)
}
func (this Configuration) DefaultedBoolOption(name string, def bool, desc string) Configuration {
	return this.addOption(name, config.BoolOption, def, desc)
}

func (this Configuration) DurationOption(name string, desc string) Configuration {
	return this.addOption(name, config.DurationOption, time.Duration(0), desc)
}
func (this Configuration) DefaultedDurationOption(name string, def time.Duration, desc string) Configuration {
	return this.addOption(name, config.DurationOption, def, desc)
}

func (this Configuration) addOption(name string, t config.OptionType, def interface{}, desc string) Configuration {
	if this.settings.configs[name] != nil {
		panic(fmt.Sprintf("option %q already defined", name))
	}
	this.settings.configs[name] = extension.NewOptionDefinition(name, t, def, desc)
	return this
}

func (this Configuration) OptionSource(name string, creator extension.OptionSourceCreator) Configuration {
	if this.settings.configsources[name] != nil {
		panic(fmt.Sprintf("option source %q already defined", name))
	}
	this.settings.configsources[name] = extension.NewOptionSourceDefinition(name, creator)
	return this
}

func (this Configuration) OptionsByExample(name string, proto config.OptionSource) Configuration {
	if this.settings.configsources[name] != nil {
		panic(fmt.Sprintf("option source %q already defined", name))
	}
	this.settings.configsources[name] = extension.NewOptionSourceDefinition(name, OptionSourceCreator(proto))
	return this
}

func (this Configuration) AddFilters(f ...ResourceFilter) Configuration {
	this.settings.resource_filters = append(this.settings.resource_filters, f...)
	return this
}
func (this Configuration) Filters(f ...ResourceFilter) Configuration {
	this.settings.resource_filters = f
	return this
}

func (this Configuration) Definition() Definition {
	return &this.settings
}

func (this Configuration) RegisterAt(registry RegistrationInterface, group ...string) error {
	return registry.RegisterController(this, group...)
}

func (this Configuration) MustRegisterAt(registry RegistrationInterface, group ...string) Configuration {
	registry.MustRegisterController(this, group...)
	return this
}

func (this Configuration) Register(group ...string) error {
	return registry.RegisterController(this, group...)
}

func (this Configuration) MustRegister(group ...string) Configuration {
	registry.MustRegisterController(this, group...)
	return this
}
