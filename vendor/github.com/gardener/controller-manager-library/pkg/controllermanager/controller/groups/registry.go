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

package groups

import "github.com/gardener/controller-manager-library/pkg/controllermanager/extension/groups"

const DEFAULT = "default"

type Definitions groups.Definitions
type Definition groups.Definition
type Registry groups.Registry
type RegistrationInterface groups.RegistrationInterface

var registry = NewRegistry()

func NewRegistry() groups.Registry {
	return groups.NewRegistry("controller")
}

func DefaultDefinitions() Definitions {
	return registry.GetDefinitions()
}

func DefaultRegistry() Registry {
	return registry
}

func Register(name string) (*groups.Configuration, error) {
	return registry.RegisterGroup(name)
}

func MustRegister(name string) *groups.Configuration {
	return registry.MustRegisterGroup(name)
}
