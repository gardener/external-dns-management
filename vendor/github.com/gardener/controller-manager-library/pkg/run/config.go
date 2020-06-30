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

package run

import (
	"io/ioutil"
	"os"
	"strings"

	"github.com/gardener/controller-manager-library/pkg/config"
	"github.com/gardener/controller-manager-library/pkg/configmain"
)

const OPTION_SOURCE = "run"

type Config struct {
	LogLevel   string
	PluginDir  string
	Namespace  string
	CPUProfile string
}

var _ config.OptionSource = (*Config)(nil)

func init() {
	configmain.RegisterExtension(func(cfg *configmain.Config) {
		cfg.AddSource(OPTION_SOURCE, &Config{})
	})
}

func (this *Config) AddOptionsToSet(set config.OptionSet) {
	namespace := "kube-system"
	n := os.Getenv("NAMESPACE")
	if n != "" {
		namespace = n
	} else {
		f := "/var/run/secrets/kubernetes.io/serviceaccount/namespace"
		bytes, err := ioutil.ReadFile(f)
		if err == nil {
			n = string(bytes)
			n = strings.TrimSpace(n)
			if n != "" {
				namespace = n
			}
		}
	}

	set.AddStringOption(&this.Namespace, "namespace", "", namespace, "namespace for lease")
	set.AddStringOption(&this.PluginDir, "plugin-file", "", "", "directory containing go plugins")
	set.AddStringOption(&this.LogLevel, "log-level", "D", "", "logrus log level")
	set.AddStringOption(&this.CPUProfile, "cpuprofile", "", "", "set file for cpu profiling")
}

func GetConfig(cfg *configmain.Config) *Config {
	return cfg.GetSource(OPTION_SOURCE).(*Config)
}
