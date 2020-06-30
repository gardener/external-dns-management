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

package mappings

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/groups"
	"github.com/gardener/controller-manager-library/pkg/utils"
)

type Definitions interface {
	Get(mtype, name string) Definition
	GetEffective(name string, grps groups.Definitions) (Definition, error)
}

const CLUSTER_MAIN = "<MAIN>"

const TYPE_GROUP = "group"
const TYPE_CONTROLLER = "controller"

type Definition interface {
	Type() string
	Name() string
	MapCluster(name string) string
	MapInfo(name string) string
	MappedClusters() utils.StringSet
	String() string
}

type _Definition struct {
	dtype    string
	name     string
	mappings map[string]string
}

func newDefinition(mtype, name string) *_Definition {
	return &_Definition{mtype, name, map[string]string{}}
}

func newDefinitionForGroup(name string) *_Definition {
	return newDefinition(TYPE_GROUP, name)
}

func newDefinitionForController(name string) *_Definition {
	return newDefinition(TYPE_CONTROLLER, name)
}

func (this *_Definition) Type() string {
	return this.dtype
}

func (this *_Definition) Name() string {
	return this.name
}

func (this *_Definition) MapCluster(name string) string {
	t := this.mappings[name]
	if t != "" {
		return t
	}
	return name
}

func (this *_Definition) MapInfo(name string) string {
	t := this.mappings[name]
	if t != "" {
		return fmt.Sprintf("%q (mapped to %q)", ClusterName(name), t)
	}
	return name
}

func (this *_Definition) MappedClusters() utils.StringSet {
	clusters := utils.StringSet{}
	for c := range this.mappings {
		clusters.Add(c)
	}
	return clusters
}

func (this *_Definition) String() string {
	return fmt.Sprintf("%v", this.mappings)[3:]
}

////////////////////////////////////////////////////////////////////////////////

type aggregation struct {
	list []Definition
}

var _ Definition = &aggregation{}

func (this *aggregation) Definition() Definition {
	return this
}

func (this *aggregation) Type() string {
	return TYPE_CONTROLLER
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

func (this *_Definitions) GetEffective(controller string, grps groups.Definitions) (Definition, error) {
	this.lock.RLock()
	defer this.lock.RUnlock()

	aggr := &aggregation{}
	direct, ok := this.definitions[TYPE_CONTROLLER][controller]
	if ok {
		aggr.list = append(aggr.list, direct)
	}

	for g, m := range this.getForType(TYPE_GROUP) {
		grp := grps.Get(g)
		if grp == nil {
			return nil, fmt.Errorf("unknown controller group %q", g)
		}
		if grp.Members().Contains(controller) {
			for cluster := range m.MappedClusters() {
				new := m.MapCluster(cluster)
				if old := aggr.MapCluster(cluster); old != cluster && old != new {
					return nil, fmt.Errorf("ambigious cluster mapping for controller %q in group %q: %q -> %q and %q",
						controller, g, cluster, old, new)
				}
			}
			aggr.list = append(aggr.list, m)
		}
	}
	return aggr, nil
}
