/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package groups

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/utils"
)

const DEFAULT = "default"

type Groups map[string]utils.StringSet

func (this Groups) String() string {
	s := "{"
	sep := ""
	for k, v := range this {
		s += fmt.Sprintf("%s%s: %s", sep, k, v)
		sep = ", "
	}
	return s + "}"
}

type Definitions interface {
	Get(name string) Definition
	Members(log logger.LogContext, elems []string) (utils.StringSet, error)
	AllGroups() Groups
	AllMembers() utils.StringSet
	AllNonExplicitMembers() utils.StringSet
}

type Definition interface {
	Members() utils.StringSet
	ActivateExplicitlyMembers() utils.StringSet
}

type _Definition struct {
	name     string
	members  utils.StringSet
	explicit utils.StringSet
}

var _ Definition = &_Definition{}

func (this *_Definition) copy() *_Definition {
	return &_Definition{name: this.name, members: this.members.Copy(),
		explicit: this.explicit.Copy()}
}

func (this *_Definition) Members() utils.StringSet {
	return this.members.Copy()
}

func (this *_Definition) ActivateExplicitlyMembers() utils.StringSet {
	return this.explicit.Copy()
}

////////////////////////////////////////////////////////////////////////////////

func (this *_Definitions) Members(log logger.LogContext, members []string) (utils.StringSet, error) {
	this.lock.RLock()
	defer this.lock.RUnlock()
	active := utils.StringSet{}
	explicitActive := utils.StringSet{}
	if len(members) == 0 {
		log.Infof("activating all %ss", this.typeName)
		active = this.AllNonExplicitMembers()
	} else {
		for _, name := range members {
			g := this.definitions[name]
			if g != nil {
				log.Infof("activating %s group %q", this.typeName, name)
				active.AddSet(g.Members().RemoveSet(g.explicit))
			} else {
				if name == "all" {
					log.Infof("activating all %ss", this.typeName)
					active.AddSet(this.AllNonExplicitMembers())
				} else {
					if this.elements.Contains(name) {
						log.Infof("activating %s %q", name, this.typeName)
						active.Add(name)
						explicitActive.Add(name)
					} else {
						return nil, fmt.Errorf("unknown %s or group %q", this.typeName, name)
					}
				}
			}
		}
	}

	log.Infof("activated %ss: %s", this.typeName, active)
	return active, nil
}

func (this *_Definitions) AllGroups() Groups {
	this.lock.RLock()
	defer this.lock.RUnlock()
	active := map[string]utils.StringSet{}
	for n, g := range this.definitions {
		active[n] = g.members.Copy()
	}
	return active
}

func (this *_Definitions) AllMembers() utils.StringSet {
	this.lock.RLock()
	defer this.lock.RUnlock()
	active := utils.StringSet{}
	for _, g := range this.definitions {
		active.AddSet(g.members)
	}
	return active
}

func (this *_Definitions) AllNonExplicitMembers() utils.StringSet {
	this.lock.RLock()
	defer this.lock.RUnlock()
	active := utils.StringSet{}
	for _, g := range this.definitions {
		active.AddSet(g.members).RemoveSet(g.explicit)
	}
	return active
}
