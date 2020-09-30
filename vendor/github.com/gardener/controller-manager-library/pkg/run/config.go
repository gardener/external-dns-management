/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
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
