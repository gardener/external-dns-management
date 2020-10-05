/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
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
