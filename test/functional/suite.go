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
