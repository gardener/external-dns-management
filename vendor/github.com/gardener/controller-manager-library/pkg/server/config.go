/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved.
 * This file is licensed under the Apache Software License, v. 2 except as noted
 * otherwise in the LICENSE file
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

package server

import (
	"context"

	"github.com/gardener/controller-manager-library/pkg/config"
	"github.com/gardener/controller-manager-library/pkg/configmain"
	"github.com/gardener/controller-manager-library/pkg/logger"
)

const OPTION_SOURCE = "http-server"

type Config struct {
	ServerPortHTTP int
	BindAddress    string
}

var _ config.OptionSource = (*Config)(nil)

func init() {
	configmain.RegisterExtension(func(cfg *configmain.Config) {
		cfg.AddSource(OPTION_SOURCE, &Config{})
	})
}

func (this *Config) AddOptionsToSet(set config.OptionSet) {
	set.AddIntOption(&this.ServerPortHTTP, "server-port-http", "", 0, "HTTP server port (serving /healthz, /metrics, ...)")
	set.AddStringOption(&this.BindAddress, "bind-address-http", "", "", "HTTP server bind address")
}

func Get(cfg *configmain.Config) *Config {
	return cfg.GetSource(OPTION_SOURCE).(*Config)
}

func ServeFromMainConfig(ctx context.Context, name string) {
	cfg := Get(configmain.Get(ctx))
	if cfg.ServerPortHTTP > 0 {
		server := NewDefaultHTTPServer(ctx, logger.New(), name)
		server.Start(nil, cfg.BindAddress, cfg.ServerPortHTTP)
	}
}
