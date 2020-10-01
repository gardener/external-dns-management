/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package controller

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	areacfg "github.com/gardener/controller-manager-library/pkg/controllermanager/controller/config"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/groups"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/mappings"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/utils"
)

type Definitions interface {
	Get(name string) Definition
	Size() int
	Names() utils.StringSet
	Groups() groups.Definitions
	GetRequiredControllers(name string) (utils.StringSet, error)
	GetMappingsFor(name string) (mappings.Definition, error)
	DetermineRequestedClusters(clusters cluster.Definitions, sets ...utils.StringSet) (_clusters utils.StringSet, _refs CrossClusterRefs, _err error)
	Registrations(names ...string) (Registrations, error)
	ExtendConfig(cfg *areacfg.Config)
}

func (this *_Definitions) Size() int {
	return len(this.definitions)
}

func (this *_Definitions) Groups() groups.Definitions {
	return this.groups
}

func (this *_Definitions) Names() utils.StringSet {
	set := utils.StringSet{}
	for n := range this.definitions {
		set.Add(n)
	}
	return set
}

func (this *_Definitions) GetRequiredControllers(name string) (utils.StringSet, error) {
	required := utils.StringSet{}
	missing := utils.StringSet{}
	this.getRequiredControllers(name, &required, &missing)
	if len(missing) > 0 {
		return nil, fmt.Errorf("controller %q required controllers %s, which are missing", name, missing)
	}
	return required, nil
}

func (this *_Definitions) getRequiredControllers(name string, required, missing *utils.StringSet) {
	if !required.Contains(name) {
		required.Add(name)
		def := this.Get(name)
		if def != nil {
			for _, d := range def.RequiredControllers() {
				this.getRequiredControllers(d, required, missing)
			}
		} else {
			missing.Add(name)
		}
	}
}

func (this *_Definitions) GetMappingsFor(name string) (mappings.Definition, error) {
	return this.mappings.GetEffective(name, this.groups)
}

func (this *_Definitions) DetermineRequestedClusters(cdefs cluster.Definitions, controllersets ...utils.StringSet) (_clusters utils.StringSet, refs CrossClusterRefs, _err error) {
	var controller_names utils.StringSet
	switch len(controllersets) {
	case 0:
		controller_names = this.definitions.Names()
	case 1:
		controller_names = controllersets[0]
	default:
		controller_names = utils.NewStringSetBySets(controllersets...)
	}
	this.lock.RLock()
	defer this.lock.RUnlock()

	clusters := utils.StringSet{}
	withids := CrossClusterRefs{}
	logger.Infof("determining required clusters:")
	logger.Infof("  found mappings: %s", this.mappings)
	for n := range controller_names {
		def := this.definitions[n]
		if def == nil {
			return nil, nil, fmt.Errorf("controller %q not definied", n)
		}
		names := cluster.Canonical(def.RequiredClusters())

		cmp, err := this.GetMappingsFor(def.Name())
		if err != nil {
			return nil, nil, err
		}

		logger.Infof("  for controller %s:", n)
		logger.Infof("     found mappings %s", cmp)
		logger.Infof("     logical clusters %s", utils.Strings(names...))
		logger.Infof("     found cross refs %s", def.CrossClusterReferences())

		set, mapping, found, err := mappings.DetermineClusterMappings(cdefs, cmp, names...)
		if err != nil {
			return nil, nil, fmt.Errorf("controller %q %s", def.Name(), err)
		}
		clusters.AddSet(set)
		logger.Infof("  mapped to %s", utils.Strings(found...))
		def.CrossClusterReferences().Map(mapping)

		withids.AddAll(def.CrossClusterReferences().Map(mapping))
	}
	return clusters, withids, nil
}

func (this *_Definitions) Registrations(names ...string) (Registrations, error) {
	this.lock.RLock()
	defer this.lock.RUnlock()
	var r = Registrations{}

	if len(names) == 0 {
		r = this.definitions.Copy()
	} else {
		for _, name := range names {
			def := this.definitions[name]
			if def == nil {
				return nil, fmt.Errorf("controller %q not found", name)
			}
			r[name] = def
		}
	}

	return r, nil
}
