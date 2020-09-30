/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package controller

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/cluster"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/utils"
)

////////////////////////////////////////////////////////////////////////////////

type controllers []*controller

func (this controllers) deps() map[string]controllers {
	deps := map[string]controllers{}

	for _, c := range this {
		n := c.GetName()
		def := c.GetDefinition()
		for _, a := range def.After() {
			if d := this.Get(a); d != nil {
				deps[n] = append(deps[n], d)
			}
		}
		for _, b := range def.Before() {
			deps[b] = append(deps[b], c)
		}
	}
	return deps
}

func (this controllers) Contains(cntr *controller) bool {
	for _, c := range this {
		if c == cntr {
			return true
		}
	}
	return false
}

func (this controllers) Get(name string) *controller {
	for _, c := range this {
		if c.GetName() == name {
			return c
		}
	}
	return nil
}

func (this controllers) getOrder(logger logger.LogContext) (controllers, error) {
	order := controllers{}
	stack := controllers{}
	for _, c := range this {
		err := this._orderAdd(logger, this.deps(), &order, stack, c)
		if err != nil {
			return nil, err
		}
	}
	return order, nil
}

func (this controllers) _orderAdd(logger logger.LogContext, depsmap map[string]controllers, order *controllers, stack controllers, c *controller) error {
	if stack.Contains(c) {
		cycle := ""
		for _, s := range stack {
			if cycle != "" || s == c {
				if cycle != "" {
					cycle += " -> "
				}
				cycle += c.GetName()
			}
		}
		return fmt.Errorf("controller startup cycle detected: %s -> %s", cycle, c.GetName())
	}
	if !(*order).Contains(c) {
		stack = append(stack, c)
		deps := depsmap[c.GetName()]
		if len(deps) > 0 {
			preferred := []string{}
			for _, dep := range deps {
				preferred = append(preferred, dep.GetName())
				err := this._orderAdd(logger, depsmap, order, stack, dep)
				if err != nil {
					return err
				}
			}
			if len(preferred) > 0 {
				logger.Infof("  %s needs to be started after %s", c.GetName(), utils.Strings(preferred...))
			}
		}
		*order = append(*order, c)
	}
	return nil
}

////////////////////////////////////////////////////////////////////////////////

type StartupGroup interface {
	Startup() error
	Add(c *controller)
	Controllers() controllers
}

type startupgroup struct {
	extension   *Extension
	cluster     cluster.Interface
	controllers controllers
}

func (this *startupgroup) Add(c *controller) {
	this.controllers = append(this.controllers, c)
}

func (this *startupgroup) Startup() error {
	if len(this.controllers) == 0 {
		return nil
	}

	msg := this.cluster.GetName()
	sep := " ("
	for _, c := range this.controllers {
		msg = fmt.Sprintf("%s%s%s", msg, sep, c.GetName())
		sep = ", "
	}
	msg += ")"

	this.extension.Infof("no lease required for %s -> starting controllers", msg)
	for _, c := range this.controllers {
		err := this.extension.startController(c)
		if err != nil {
			return err
		}
	}
	return nil
}

func (this *startupgroup) Controllers() controllers {
	return this.controllers
}

////////////////////////////////////////////////////////////////////////////////

func (this *Extension) getPlainStartupGroup(cluster cluster.Interface) StartupGroup {
	g := this.plain_groups[cluster.GetName()]
	if g == nil {
		g = &startupgroup{this, cluster, nil}
		this.plain_groups[cluster.GetName()] = g
	}
	return g
}

func (this *Extension) getLeaseStartupGroup(cluster cluster.Interface) StartupGroup {
	g := this.lease_groups[cluster.GetName()]
	if g == nil {
		g = &leasestartupgroup{startupgroup{this, cluster, nil}}
		this.lease_groups[cluster.GetName()] = g
	}
	return g
}

func (this *Extension) startGroups(grps ...map[string]StartupGroup) error {
	for _, grp := range grps {
		for _, g := range grp {
			err := g.Startup()
			if err != nil {
				return err
			}
		}
	}
	return nil
}
