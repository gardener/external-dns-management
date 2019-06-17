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

package functional

import (
	"github.com/gardener/external-dns-management/test/functional/config"
	"sync"
)

var _config *config.Config
var lock sync.Mutex

func addProviderTests(testFactory ProviderTestFactory) {
	lock.Lock()
	defer lock.Unlock()

	if _config == nil {
		_config = config.InitConfig()
	}

	for _, provider := range _config.Providers {
		testFactory(_config, provider)
	}
}

type ProviderTestFactory func(config *config.Config, provider *config.ProviderConfig)
