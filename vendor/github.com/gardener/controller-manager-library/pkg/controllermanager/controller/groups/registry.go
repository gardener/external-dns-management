/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
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
