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

package config

import (
	"fmt"
	"reflect"
	"sync"

	"github.com/spf13/cobra"
)

type Config struct {
	lock             sync.Mutex
	LogLevel         string
	Controllers      string
	PluginDir        string
	ServerPortHTTP   int
	ArbitraryOptions map[string]*ArbitraryOption
}

func NewConfig() *Config {
	config := &Config{}
	config.ArbitraryOptions = map[string]*ArbitraryOption{}
	addExtensions(config)
	return config
}

func (this *Config) GetOption(name string) *ArbitraryOption {
	this.lock.Lock()
	defer this.lock.Unlock()
	return this.ArbitraryOptions[name]
}

func (this *Config) addOption(name string) (*ArbitraryOption, bool) {
	this.lock.Lock()
	defer this.lock.Unlock()

	c := this.ArbitraryOptions[name]
	if c == nil {
		c = &ArbitraryOption{Name: name}
		this.ArbitraryOptions[name] = c
		return c, true
	}
	return c, false
}

func (this *Config) AddOption(name string, t reflect.Type) (*ArbitraryOption, bool) {
	opt, new := this.addOption(name)

	if new {
		opt.Type = t
	} else {
		if opt.Type != t {
			panic(fmt.Sprintf("non matching option type for %q", name))
		}
	}
	return opt, new
}

func (this *Config) AddStringOption(name string) (*ArbitraryOption, bool) {
	return this.AddOption(name, reflect.TypeOf((*string)(nil)).Elem())
}

func (this *Config) AddStringArrayOption(name string) (*ArbitraryOption, bool) {
	return this.AddOption(name, reflect.TypeOf(([]string)(nil)))
}

func (this *Config) AddIntOption(name string) (*ArbitraryOption, bool) {
	return this.AddOption(name, reflect.TypeOf((*int)(nil)).Elem())
}

func (this *Config) AddToCommand(cmd *cobra.Command) {
	cmd.PersistentFlags().StringVarP(&this.Controllers, "controllers", "", "all", "comma separated list of controllers to start (<name>,source,target,all)")
	cmd.PersistentFlags().StringVarP(&this.PluginDir, "plugin-dir", "", "", "directory containing go plugins")
	cmd.PersistentFlags().IntVarP(&this.ServerPortHTTP, "server-port-http", "", 0, "directory containing go plugins")
	cmd.PersistentFlags().StringVarP(&this.LogLevel, "log-level", "D", "", "logrus log level")

	for _, o := range this.ArbitraryOptions {
		o.AddToCommand(cmd)
	}
}
