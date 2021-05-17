/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 */

package watches

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/extension"
	"github.com/gardener/controller-manager-library/pkg/resources"
)

// ResourceKey implementations are used as key and MUST therefore be value types
type ResourceKey = extension.ResourceKey

// WatchContext describes the context to evaluate a resource key for
type WatchContext interface {
	Name() string
	extension.ElementOptions
	Cluster() cluster.Interface
	Namespace() string
}

type WatchResourceDef struct {
	Key       ResourceKey
	Namespace string
	Tweaker   []resources.TweakListOptionsFunc
	Minimal   bool
}

////////////////////////////////////////////////////////////////////////////////
// Flavored Resource

// FlavoredResource describe a sequence of potential Resource Flavors
// which is checked to determine the actual ressource key for a dedicated
// watch context. The first flavor yielding a result will be chosen.
type FlavoredResource []ResourceFlavor

func (this FlavoredResource) RequestMinimalFor(gk schema.GroupKind) {
	for _, f := range this {
		f.RequestMinimalFor(gk)
	}
}

func (this FlavoredResource) WatchResourceDef(wctx WatchContext, def WatchResourceDef) WatchResourceDef {
	for _, r := range this {
		def = r.WatchResourceDef(wctx, def)
		if def.Key != nil {
			break
		}
	}
	return def
}

func (this FlavoredResource) String() string {
	s := "["
	sep := ""
	for _, f := range this {
		s = fmt.Sprintf("%s%s%s", s, sep, f)
		sep = ", "
	}
	return s + "]"
}

func NewFlavoredResource(flavors ...ResourceFlavor) FlavoredResource {
	return flavors
}

// ResourceFlavor describes a dedicated flavor a resource
// If it matches a dedicated watch context a resource key is returned,
// nil otherwise
type ResourceFlavor interface {
	WatchResourceDef(wctx WatchContext, def WatchResourceDef) WatchResourceDef
	RequestMinimalFor(gk schema.GroupKind)
	String() string
}
