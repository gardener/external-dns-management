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

package config

import (
	"fmt"
	"time"

	"github.com/gardener/controller-manager-library/pkg/config"
	"github.com/gardener/controller-manager-library/pkg/configmain"
	"github.com/gardener/controller-manager-library/pkg/utils"
)

var GracePeriod time.Duration

const OPTION_SOURCE = "controllermanager"

type MaintainerInfo struct {
	Ident          string
	Idents         utils.StringSet
	ForceCRDUpdate bool
}

const CRD_UNMANAGED = "UNMANAGED"

type Config struct {
	Name                        string
	DisableNamespaceRestriction bool
	NamespaceRestriction        bool

	idents        string
	CRDMaintainer MaintainerInfo

	config.OptionSet
}

var _ config.OptionSource = (*Config)(nil)

func NewConfig(name string) *Config {
	cfg := &Config{
		OptionSet: config.NewDefaultOptionSet(OPTION_SOURCE, ""),
	}
	cfg.AddDurationOption(&GracePeriod, "grace-period", "", 0, "inactivity grace period for detecting end of cleanup for shutdown")
	cfg.AddStringOption(&cfg.Name, "name", "", name, "name used for controller manager")
	cfg.AddBoolOption(&cfg.NamespaceRestriction, "namespace-local-access-only", "n", false, "enable access restriction for namespace local access only (deprecated)")
	cfg.AddBoolOption(&cfg.DisableNamespaceRestriction, "disable-namespace-restriction", "", false, "disable access restriction for namespace local access only")
	cfg.AddStringOption(&cfg.idents, "accepted-maintainers", "", "", "accepted maintainer key(s) for crds")
	cfg.AddStringOption(&cfg.CRDMaintainer.Ident, "maintainer", "", name, "maintainer key for crds")
	cfg.AddBoolOption(&cfg.CRDMaintainer.ForceCRDUpdate, "force-crd-update", "", false, "enforce update of crds even they are unmanaged")
	return cfg
}

func (this *Config) Evaluate() error {
	if this.NamespaceRestriction && this.DisableNamespaceRestriction {
		return fmt.Errorf("contradicting options given for namespace restriction")
	}
	if !this.DisableNamespaceRestriction {
		this.NamespaceRestriction = true
	}
	this.DisableNamespaceRestriction = false
	if this.CRDMaintainer.Ident == "" {
		this.CRDMaintainer.Ident = this.Name
	}
	this.CRDMaintainer.Idents = utils.NewStringSet().AddAllSplittedSelected(this.idents, utils.NonEmptyStringElement)
	if this.CRDMaintainer.Idents.Contains(CRD_UNMANAGED) {
		this.CRDMaintainer.Idents.Add("")
		this.CRDMaintainer.Idents.Remove(CRD_UNMANAGED)
	}
	this.CRDMaintainer.Idents.Add(this.CRDMaintainer.Ident)
	return this.OptionSet.Evaluate()
}

func GetConfig(cfg *configmain.Config) *Config {
	return cfg.GetSource(OPTION_SOURCE).(*Config)
}
