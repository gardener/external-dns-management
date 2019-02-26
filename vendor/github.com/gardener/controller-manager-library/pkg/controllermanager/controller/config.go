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
	"reflect"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/config"
)

func ControllerOption(controller, name string) string {
	return fmt.Sprintf("%s.%s", controller, name)
}
func PoolSizeOptionName(controller, pool string) string {
	return fmt.Sprintf("%s.%s.%s", controller, pool, POOL_SIZE_OPTION)
}

func PoolResyncPeriodOptionName(controller, pool string) string {
	return fmt.Sprintf("%s.%s.%s", controller, pool, POOL_RESYNC_PERIOD_OPTION)
}

const POOL_SIZE_OPTION = "pool.size"
const POOL_RESYNC_PERIOD_OPTION = "pool.resync-period"

func (this *_Definitions) ExtendConfig(cfg *config.Config) {
	shared := map[string]reflect.Type{}

	updateSharedOption := func(name string, opt *config.ArbitraryOption) {
		old, ok := shared[name]
		if !ok || old == opt.Type {
			shared[name] = opt.Type
		} else {
			shared[name] = nil
		}
	}

	for name, def := range this.definitions {
		for pname, p := range def.Pools() {
			opt, _ := cfg.AddIntOption(PoolSizeOptionName(name, pname))
			opt.Description = fmt.Sprintf("Worker pool size for pool %s of controller %s (default: %d)", pname, name, p.Size())
			opt.Default = p.Size()
			updateSharedOption(POOL_SIZE_OPTION, opt)

			if p.Period() != 0 {
				opt, _ := cfg.AddDurationOption(PoolResyncPeriodOptionName(name, pname))
				opt.Description = fmt.Sprintf("Period for resynchronization of pool %s of controller %s (default: %s)",
					pname, name, p.Period().String())
				opt.Default = p.Period()
				updateSharedOption(POOL_RESYNC_PERIOD_OPTION, opt)
			}
		}

		for oname, o := range def.ConfigOptions() {
			opt, _ := cfg.AddOption(ControllerOption(name, oname), o.Type())
			opt.Description = o.Description()
			opt.Default = o.Default()
			updateSharedOption(oname, opt)
		}
	}

	this.shared = map[string]*config.ArbitraryOption{}
	for o, t := range shared {
		if t != nil {
			opt, _ := cfg.AddOption(o, t)
			opt.Description = fmt.Sprintf("default for all controller %q options", o)
			this.shared[o] = opt
		}
	}
}
