/*
 * Copyright 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 *
 */

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
