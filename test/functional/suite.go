// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package functional

import (
	"sync"

	"github.com/gardener/external-dns-management/test/functional/config"
)

var (
	_config *config.Config
	lock    sync.Mutex
)

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
