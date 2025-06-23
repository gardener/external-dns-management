// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package pprof

import (
	"net/http"
	"net/http/pprof"

	"github.com/gardener/controller-manager-library/pkg/config"
	"github.com/gardener/controller-manager-library/pkg/configmain"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/server"
)

const OPTION_SOURCE = "profiling"

type Config struct {
	Enabled bool
}

var _ config.OptionSource = (*Config)(nil)

func init() {
	configmain.RegisterExtension(func(cfg *configmain.Config) {
		cfg.AddSource(OPTION_SOURCE, &Config{})
	})
}

func (this *Config) AddOptionsToSet(set config.OptionSet) {
	set.AddBoolOption(&this.Enabled, "enable-profiling", "", false, "enables profiling server at path /debug/pprof (needs option --server-port-http)")
}

func (this *Config) Evaluate() error {
	if this.Enabled {
		logger.New().Info("enabled profiling endpoints at /debug/pprof")
		AddProfilingHandlers()
	}
	return nil
}

func AddProfilingHandlers() {
	server.RegisterHandler("/debug/pprof", http.HandlerFunc(redirectTo("/debug/pprof/")))
	server.RegisterHandler("/debug/pprof/", http.HandlerFunc(pprof.Index))
	server.RegisterHandler("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
	server.RegisterHandler("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
	server.RegisterHandler("/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
	server.RegisterHandler("/debug/pprof/trace", http.HandlerFunc(pprof.Trace))
}

// redirectTo redirects request to a certain destination.
func redirectTo(to string) func(http.ResponseWriter, *http.Request) {
	return func(rw http.ResponseWriter, req *http.Request) {
		http.Redirect(rw, req, to, http.StatusFound)
	}
}
