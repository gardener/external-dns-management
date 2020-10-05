/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package controller

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/config"
	areacfg "github.com/gardener/controller-manager-library/pkg/controllermanager/controller/config"
)

type ControllerConfig struct {
	*config.GenericOptionSource
}

func NewControllerConfig(controller string) *ControllerConfig {
	return &ControllerConfig{
		GenericOptionSource: config.NewGenericOptionSource(controller, controller, func(desc string) string {
			return fmt.Sprintf("%s of controller %s", desc, controller)
		}),
	}
}

const POOL_SIZE_OPTION = "pool.size"
const POOL_RESYNC_PERIOD_OPTION = "pool.resync-period"

const CONTROLLER_SET_PREFIX = "controller."

func (this *_Definitions) ExtendConfig(cfg *areacfg.Config) {

	for name, def := range this.definitions {
		ccfg := NewControllerConfig(name)
		cfg.AddSource(name, ccfg)

		set := ccfg.PrefixedShared()

		for pname, p := range def.Pools() {
			localpname := pname
			pcfg := config.NewSharedOptionSet(pname, pname, func(s string) string {
				return s + " for pool " + localpname
			})
			set.AddSource(pname, pcfg)
			pcfg.AddIntOption(nil, POOL_SIZE_OPTION, "", p.Size(), "Worker pool size")

			if p.Period() != 0 {
				pcfg.AddDurationOption(nil, POOL_RESYNC_PERIOD_OPTION, "", p.Period(), "Period for resynchronization")
			}
		}

		for oname, o := range def.ConfigOptions() {
			set.AddOption(o.Type(), nil, oname, "", o.Default(), o.Description())
		}
		for oname, o := range def.ConfigOptionSources() {
			if src := o.Create(); src != nil {
				set.AddSource(CONTROLLER_SET_PREFIX+oname, src)
			}
		}
	}
}
