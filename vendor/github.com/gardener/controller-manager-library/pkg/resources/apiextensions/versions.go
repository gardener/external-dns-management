/*
 * SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 */

package apiextensions

import (
	"fmt"

	"github.com/Masterminds/semver"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/extension"
	"github.com/gardener/controller-manager-library/pkg/logger"
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

func (this *CustomResourceDefinitionVersions) Deploy(log logger.LogContext, cluster cluster.Interface, maintainer extension.MaintainerInfo) error {
	crd := this.GetFor(cluster.GetServerVersion())
	if crd != nil {
		err := CreateCRDFromObject(log, cluster, crd.DataFor(cluster, nil), maintainer)
		if err != nil {
			return err
		}
	}
	return nil
}
