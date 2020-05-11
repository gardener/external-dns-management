/*
 * Copyright 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *       http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 *
 */

package apiextensions

import (
	"fmt"

	"github.com/Masterminds/semver"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/gardener/controller-manager-library/pkg/resources/abstract"
	reserr "github.com/gardener/controller-manager-library/pkg/resources/errors"
	"github.com/gardener/controller-manager-library/pkg/utils"
)

type CRDVersion string

const CRD_V1 = CRDVersion("v1")
const CRD_V1BETA1 = CRDVersion("v1beta1")

type CustomResourceDefinitionVersions struct {
	groupkind schema.GroupKind
	versioned *utils.Versioned
}

var v116 = semver.MustParse("1.16.0")
var v112 = semver.MustParse("1.12.0")
var otype runtime.Object

func NewDefaultedCustomResourceDefinitionVersions(spec CRDSpecification) (*CustomResourceDefinitionVersions, error) {
	var def *CustomResourceDefinitionVersions

	switch s := spec.(type) {
	case abstract.GroupKindProvider:
		{
			def = registry.GetCRDs(s.GroupKind())
			if def == nil {
				return nil, reserr.ErrNotFound.New("CRD", s.GroupKind())
			}
		}
	case schema.GroupKind:
		def = registry.GetCRDs(s)
		if def == nil {
			return nil, reserr.ErrNotFound.New("CRD", spec)
		}
	default:
		o, err := GetCustomResourceDefinition(spec)
		if err != nil {
			return nil, err
		}
		def = NewCustomResourceDefinitionVersions(o.CRDGroupKind())
		def.versioned.SetDefault(o)
	}
	return def, nil
}

func NewCustomResourceDefinitionVersions(gk schema.GroupKind) *CustomResourceDefinitionVersions {
	return &CustomResourceDefinitionVersions{gk, utils.NewVersioned(&CustomResourceDefinition{})}

}

func (this *CustomResourceDefinitionVersions) Name() string {
	def := this.GetDefault()
	if def != nil {
		return def.Name
	}
	for _, o := range this.versioned.GetVersions() {
		return o.(*CustomResourceDefinition).Name
	}
	return ""
}

func (this *CustomResourceDefinitionVersions) GroupKind() schema.GroupKind {
	return this.groupkind
}

func (this *CustomResourceDefinitionVersions) GetFor(vers *semver.Version) *CustomResourceDefinition {
	f := this.versioned.GetFor(vers)
	if f != nil {
		return f.(*CustomResourceDefinition)
	}
	return nil
}

func (this *CustomResourceDefinitionVersions) GetDefault() *CustomResourceDefinition {
	f := this.versioned.GetDefault()
	if f != nil {
		return f.(*CustomResourceDefinition)
	}
	return nil
}

func (this *CustomResourceDefinitionVersions) GetVersions() map[*semver.Version]*CustomResourceDefinition {
	result := map[*semver.Version]*CustomResourceDefinition{}
	for v, o := range this.versioned.GetVersions() {
		result[v] = o.(*CustomResourceDefinition)
	}
	return result
}

func (this *CustomResourceDefinitionVersions) Override(v *semver.Version, spec CRDSpecification) *CustomResourceDefinitionVersions {
	crd, err := GetCustomResourceDefinition(spec)
	utils.Must(err)
	name := this.Name()
	if name != "" && crd.Name != name {
		panic(fmt.Errorf("crd name mismatch %s != %s", name, crd.Name))
	}
	this.versioned.MustRegisterVersion(v, crd)
	return this
}
