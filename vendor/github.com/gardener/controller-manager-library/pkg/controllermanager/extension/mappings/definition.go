/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package mappings

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/extension/groups"
	"github.com/gardener/controller-manager-library/pkg/utils"
)

type Definitions interface {
	Get(mtype, name string) Definition
	GetEffective(name string, grps groups.Definitions) (Definition, error)
}

const CLUSTER_MAIN = "<MAIN>"

const TYPE_GROUP = "group"

type Definition interface {
	Type() string
	Name() string
	MapCluster(name string) string
	MapInfo(name string) string
	MappedClusters() utils.StringSet
	String() string
}

type DefinitionImpl struct {
	dtype    string
	name     string
	mappings map[string]string
}

func NewDefinition(mtype, name string) *DefinitionImpl {
	return &DefinitionImpl{mtype, name, map[string]string{}}
}

func (this *DefinitionImpl) Type() string {
	return this.dtype
}

func (this *DefinitionImpl) Name() string {
	return this.name
}

func (this *DefinitionImpl) MapCluster(name string) string {
	t := this.mappings[name]
	if t != "" {
		return t
	}
	return name
}

func (this *DefinitionImpl) MapInfo(name string) string {
	t := this.mappings[name]
	if t != "" {
		return fmt.Sprintf("%q (mapped to %q)", ClusterName(name), t)
	}
	return name
}

func (this *DefinitionImpl) MappedClusters() utils.StringSet {
	clusters := utils.StringSet{}
	for c := range this.mappings {
		clusters.Add(c)
	}
	return clusters
}

func (this *DefinitionImpl) String() string {
	return fmt.Sprintf("%v", this.mappings)[3:]
}

/// internal

func (this *DefinitionImpl) Copy() {
	new := map[string]string{}
	for k, v := range this.mappings {
		new[k] = v
	}
	this.mappings = new
}

func (this *DefinitionImpl) SetMapping(cluster, to string) {
	this.mappings[cluster] = to
}

////////////////////////////////////////////////////////////////////////////////

type aggregation struct {
	elemType string
	list     []Definition
}

var _ Definition = &aggregation{}

func (this *aggregation) Definition() Definition {
	return this
}

func (this *aggregation) Type() string {
	return this.elemType
}

func (this *aggregation) Name() string {
	return "<aggregated>"
}

func (this *aggregation) MapCluster(name string) string {
	mapped := name
	for _, d := range this.list {
		m := d.MapCluster(name)
		if m != name {
			mapped = m
		}
	}
	return mapped
}

func (this *aggregation) MapInfo(name string) string {
	mapped := this.MapCluster(name)
	if mapped != name {
		return fmt.Sprintf("%q (mapped to %q)", ClusterName(name), mapped)
	}
	return name
}

func (this *aggregation) MappedClusters() utils.StringSet {
	set := utils.StringSet{}
	for _, a := range this.list {
		set.AddSet(a.MappedClusters())
	}
	return set
}

func (this *aggregation) String() string {
	s := "["
	sep := ""
	for _, m := range this.list {
		s = s + sep + m.String()
		sep = ", "
	}
	return s + "]"
}

///////////////////////////////////////////////////////////////////////////////

func (this *_Definitions) GetEffective(elem string, grps groups.Definitions) (Definition, error) {
	this.lock.RLock()
	defer this.lock.RUnlock()

	aggr := &aggregation{elemType: this.elemType}
	direct, ok := this.definitions[this.elemType][elem]
	if ok {
		aggr.list = append(aggr.list, direct)
	}

	for g, m := range this.getForType(TYPE_GROUP) {
		grp := grps.Get(g)
		if grp == nil {
			return nil, fmt.Errorf("unknown %s group %q", this.elemType, g)
		}
		if grp.Members().Contains(elem) {
			for cluster := range m.MappedClusters() {
				new := m.MapCluster(cluster)
				if old := aggr.MapCluster(cluster); old != cluster && old != new {
					return nil, fmt.Errorf("ambigious cluster mapping for %s %q in group %q: %q -> %q and %q",
						this.elemType, elem, g, cluster, old, new)
				}
			}
			aggr.list = append(aggr.list, m)
		}
	}
	return aggr, nil
}
