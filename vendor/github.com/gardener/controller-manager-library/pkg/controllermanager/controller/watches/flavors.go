/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 */

package watches

import (
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/extension"
	"github.com/gardener/controller-manager-library/pkg/resources"
)

////////////////////////////////////////////////////////////////////////////////
// Resource Flavors

type dummyFlavor struct{}

func (dummyFlavor) WatchResourceDef(wctx WatchContext, def WatchResourceDef) WatchResourceDef {
	return def
}

func (dummyFlavor) RequestMinimalFor(gk schema.GroupKind) {
}

// Conditional decribes a resource flavor checked if a dedicated contraint is met.
func Conditional(cond WatchConstraint, flavors ...ResourceFlavor) ResourceFlavor {
	return &conditional{
		cond:    cond,
		flavors: FlavoredResource(flavors),
	}
}

type conditional struct {
	cond    WatchConstraint
	flavors FlavoredResource
}

func (this *conditional) WatchResourceDef(wctx WatchContext, def WatchResourceDef) WatchResourceDef {
	if this.cond == nil || this.cond.Check(wctx) {
		return this.flavors.WatchResourceDef(wctx, def)
	}
	return def
}

func (this *conditional) RequestMinimalFor(gk schema.GroupKind) {
	this.flavors.RequestMinimalFor(gk)
}
func (this *conditional) String() string {
	s := ""
	if this.cond != nil {
		s = fmt.Sprintf("IF(%s)", this.cond)
	}
	return fmt.Sprintf("%s%s", s, this.flavors)
}

//
// Minimal flavor
//

func Minimal() ResourceFlavor {
	return &minimalFlavor{}
}

type minimalFlavor struct{ dummyFlavor }

func (*minimalFlavor) WatchResourceDef(wctx WatchContext, def WatchResourceDef) WatchResourceDef {
	def.Minimal = true
	return def
}
func (this *minimalFlavor) String() string {
	return "{minimal}"
}

//
// Namespace flavor
//

func Namespace(namespace string) ResourceFlavor {
	return &namespaceFlavor{namespace: namespace}
}

type namespaceFlavor struct {
	dummyFlavor
	namespace string
}

func (this *namespaceFlavor) WatchResourceDef(wctx WatchContext, def WatchResourceDef) WatchResourceDef {
	def.Namespace = this.namespace
	return def
}
func (this *namespaceFlavor) String() string {
	return "{namespace=" + this.namespace + "}"
}

//
// Local Namespace flavor
//

func LocalNamespace() ResourceFlavor {
	return &localNamespaceFlavor{}
}

type localNamespaceFlavor struct {
	dummyFlavor
}

func (this *localNamespaceFlavor) String() string {
	return "{local namespace}"
}

//
// Namespace By Option flavor
//

func NamespaceByOption(opt string, srcnames ...string) ResourceFlavor {
	return &namespaceByOptionFlavor{
		opt:      opt,
		srcnames: srcnames,
	}
}

type namespaceByOptionFlavor struct {
	dummyFlavor
	opt      string
	srcnames []string
}

func (this *namespaceByOptionFlavor) WatchResourceDef(wctx WatchContext, def WatchResourceDef) WatchResourceDef {
	def.Namespace = getStringOptionValue(wctx, this.opt, this.srcnames...)
	return def
}
func (this *namespaceByOptionFlavor) String() string {
	return "{namespace option " + this.opt + "}"
}

//
// ListOptionTweaker flavor
//

type WatchTweaker func(wxtc WatchContext) resources.TweakListOptionsFunc

func (this WatchTweaker) WatchResourceDef(wctx WatchContext, def WatchResourceDef) WatchResourceDef {
	tweak := this(wctx)
	if tweak != nil {
		def.Tweaker = append(def.Tweaker, tweak)
	}
	return def
}
func (WatchTweaker) RequestMinimalFor(gk schema.GroupKind) {
}
func (this WatchTweaker) String() string {
	// TODO print function: fmt.Sprintf("{tweaker %v}", this) does not work
	return fmt.Sprintf("{tweaker}")
}

//
// Single Object Flavors
//

type ObjectSelector func(wctx WatchContext) resources.ObjectName

func (this ObjectSelector) WatchResourceDef(wctx WatchContext, def WatchResourceDef) WatchResourceDef {
	name := this(wctx)
	if name != nil {
		def.Tweaker = append(def.Tweaker, func(options *metav1.ListOptions) {
			options.FieldSelector = fields.OneTermEqualSelector("metadata.name", name.Name()).String()
		})
	}
	return def
}
func (ObjectSelector) RequestMinimalFor(gk schema.GroupKind) {
}
func (this ObjectSelector) String() string {
	// TODO print function
	return fmt.Sprintf("{object selector}")
}

type objectFlavor struct {
	dummyFlavor
	selector ObjectSelector
}

func (this *objectFlavor) WatchResourceDef(wctx WatchContext, def WatchResourceDef) WatchResourceDef {
	name := this.selector(wctx)
	if name != nil {
		def.Tweaker = append(def.Tweaker, func(options *metav1.ListOptions) {
			options.FieldSelector = fields.OneTermEqualSelector("metadata.name", name.Name()).String()
		})
	}
	return def
}

//
// Single Local Object Flavors
//

func LocalObjectByName(name string) ResourceFlavor {
	return &localObjectFlavor{
		objectFlavor: objectFlavor{
			selector: func(wctx WatchContext) resources.ObjectName {
				return resources.NewObjectName(wctx.Namespace(), name)
			},
		},
		name: name,
	}
}

type localObjectFlavor struct {
	objectFlavor
	name string
}

func (this *localObjectFlavor) String() string {
	// TODO print struct
	return fmt.Sprintf("{local object}")
}

//
// Local Object By Option

func ObjectByNameOption(opt string, srcnames ...string) ResourceFlavor {
	return &optionObjectFlavor{
		objectFlavor: objectFlavor{
			selector: func(wctx WatchContext) resources.ObjectName {
				return resources.NewObjectName(wctx.Namespace(), getStringOptionValue(wctx, opt, srcnames...))
			},
		},
		opt: opt,
	}
}

type optionObjectFlavor struct {
	objectFlavor
	opt string
}

func (this *optionObjectFlavor) String() string {
	return fmt.Sprintf("{local object option %s}", this.opt)
}

//
// ResourceFlavor is a single flavor valid for a dedicated constraint
//

func ResourceFlavorByGK(gk schema.GroupKind, constraints ...WatchConstraint) ResourceFlavor {
	return NewResourceFlavor(gk.Group, gk.Kind, constraints...)
}

func NewResourceFlavor(group, kind string, constraints ...WatchConstraint) ResourceFlavor {
	return &resourceFlavor{
		key:         extension.NewResourceKey(group, kind),
		constraints: constraints,
	}
}

type resourceFlavor struct {
	constraints []WatchConstraint
	key         ResourceKey
	minimal     bool
}

func (this *resourceFlavor) WatchResourceDef(wctx WatchContext, def WatchResourceDef) WatchResourceDef {
	if len(this.constraints) == 0 {
		def.Key = this.key
		if this.minimal {
			def.Minimal = true
		}
		return def
	}
	for _, c := range this.constraints {
		if c.Check(wctx) {
			def.Key = this.key
			return def
		}
	}
	return def
}
func (this *resourceFlavor) RequestMinimalFor(gk schema.GroupKind) {
	if gk == this.key.GroupKind() {
		this.minimal = true
	}
}

func (this *resourceFlavor) String() string {
	if len(this.constraints) == 0 {
		return this.key.String()
	}
	s := fmt.Sprintf("%s[", this.key)
	sep := ""
	for _, c := range this.constraints {
		s = fmt.Sprintf("%s%s%s", s, sep, c)
		sep = ", "
	}
	if this.minimal {
		return s + "]{minimal}"
	}
	return s + "]"
}

////////////////////////////////////////////////////////////////////////////////
// utils

func SimpleResourceFlavors(group, kind string, constraints ...WatchConstraint) FlavoredResource {
	return FlavoredResource{NewResourceFlavor(group, kind, constraints...)}
}

func SimpleResourceFlavorsByKey(key ResourceKey, constraints ...WatchConstraint) FlavoredResource {
	return FlavoredResource{ResourceFlavorByGK(key.GroupKind(), constraints...)}
}

func SimpleResourceFlavorsByGK(gk schema.GroupKind, constraints ...WatchConstraint) FlavoredResource {
	return FlavoredResource{ResourceFlavorByGK(gk, constraints...)}
}
