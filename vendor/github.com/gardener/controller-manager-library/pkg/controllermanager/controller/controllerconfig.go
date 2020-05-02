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
