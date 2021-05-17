/*
 * SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 */

package reconcilers

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/controller-manager-library/pkg/utils"
)

type ReconcilerSupport struct {
	controller controller.Interface
	actions    ActionDefinition
}

var _ reconcile.Interface = &ReconcilerSupport{}

func NewReconcilerSupport(c controller.Interface, actions ...ActionDefinition) ReconcilerSupport {
	if len(actions) == 0 {
		return ReconcilerSupport{controller: c}
	}
	if len(actions) == 1 {
		return ReconcilerSupport{controller: c, actions: actions[0]}
	}
	panic("invalid actions settings")
}

func (this *ReconcilerSupport) Controller() controller.Interface {
	return this.controller
}

func (this *ReconcilerSupport) Actions() ActionDefinition {
	return this.actions
}

func (this *ReconcilerSupport) EnqueueKeys(keys resources.ClusterObjectKeySet) {
	for key := range keys {
		this.Controller().EnqueueKey(key)
	}
}

func (this *ReconcilerSupport) EnqueueObject(gk schema.GroupKind, name resources.ObjectName, cluster ...string) error {
	key := this.NewClusterObjectKey(gk, name, cluster...)
	if key.Cluster() == "" {
		return fmt.Errorf("unknown cluster")
	}
	return this.controller.EnqueueKey(key)
}

func (this *ReconcilerSupport) EnqueueObjectReferencedBy(obj resources.Object, gk schema.GroupKind, name resources.ObjectName) error {
	key := resources.NewClusterKey(obj.GetCluster().GetId(), gk, name.Namespace(), name.Name())
	return this.controller.EnqueueKey(key)
}

func (this *ReconcilerSupport) NewClusterObjectKey(gk schema.GroupKind, name resources.ObjectName, cluster ...string) resources.ClusterObjectKey {
	if len(cluster) == 0 {
		return resources.NewClusterKey(this.Controller().GetMainCluster().GetId(), gk, name.Namespace(), name.Name())
	}
	c := this.controller.GetCluster(cluster[0])
	if c == nil {
		return resources.NewClusterKey("", gk, name.Namespace(), name.Name())
	}
	return resources.NewClusterKey(c.GetId(), gk, name.Namespace(), name.Name())
}

func (r *ReconcilerSupport) Reconcile(logger logger.LogContext, obj resources.Object) reconcile.Status {
	if r.actions != nil {
		return r.actions.Reconcile(logger, obj)
	}
	return reconcile.Succeeded(logger)
}
func (r *ReconcilerSupport) Delete(logger logger.LogContext, obj resources.Object) reconcile.Status {
	if r.actions != nil {
		return r.actions.Delete(logger, obj)
	}
	return reconcile.Succeeded(logger)
}
func (r *ReconcilerSupport) Deleted(logger logger.LogContext, key resources.ClusterObjectKey) reconcile.Status {
	if r.actions != nil {
		return r.actions.Deleted(logger, key)
	}
	return reconcile.Succeeded(logger)
}

func (r *ReconcilerSupport) Command(logger logger.LogContext, cmd string) reconcile.Status {
	if r.actions != nil {
		return r.actions.Command(logger, cmd)
	}
	return reconcile.Failed(logger, fmt.Errorf("unknown command %q", cmd))
}

////////////////////////////////////////////////////////////////////////////////

type ReconcileFunction func(logger logger.LogContext, obj resources.Object) reconcile.Status
type DeleteFunction func(logger logger.LogContext, obj resources.Object) reconcile.Status
type DeletedFunction func(logger logger.LogContext, key resources.ClusterObjectKey) reconcile.Status
type CommandFunction func(logger logger.LogContext, cmd string) reconcile.Status

type matcher struct {
	utils.Matcher
	Command CommandFunction
}

type ActionDefinition interface {
	reconcile.Interface

	ReconcileWith(gk schema.GroupKind, function ReconcileFunction)
	DeleteWith(gk schema.GroupKind, function DeleteFunction)
	DeletedWith(gk schema.GroupKind, function DeletedFunction)
	CommandWith(cmd string, function CommandFunction)
	MatchingCommandWith(cmd utils.Matcher, function CommandFunction)
}

type actionDefinition struct {
	reconcilers map[schema.GroupKind]ReconcileFunction
	deleters    map[schema.GroupKind]DeleteFunction
	deleteders  map[schema.GroupKind]DeletedFunction
	commands    map[string]CommandFunction
	matchers    []matcher
}

func NewActionDefinition() ActionDefinition {
	return &actionDefinition{
		reconcilers: map[schema.GroupKind]ReconcileFunction{},
		deleters:    map[schema.GroupKind]DeleteFunction{},
		deleteders:  map[schema.GroupKind]DeletedFunction{},
		commands:    map[string]CommandFunction{},
	}
}

func (r *actionDefinition) ReconcileWith(gk schema.GroupKind, function ReconcileFunction) {
	r.reconcilers[gk] = function
}
func (r *actionDefinition) DeleteWith(gk schema.GroupKind, function DeleteFunction) {
	r.deleters[gk] = function
}
func (r *actionDefinition) DeletedWith(gk schema.GroupKind, function DeletedFunction) {
	r.deleteders[gk] = function
}
func (r *actionDefinition) CommandWith(cmd string, function CommandFunction) {
	r.commands[cmd] = function
}
func (r *actionDefinition) MatchingCommandWith(cmd utils.Matcher, function CommandFunction) {
	r.matchers = append(r.matchers, matcher{cmd, function})
}

func (r *actionDefinition) ReconcileFunction(obj resources.Object) ReconcileFunction {
	return r.reconcilers[obj.GroupKind()]
}
func (r *actionDefinition) DeleteFunction(obj resources.Object) DeleteFunction {
	return r.deleters[obj.GroupKind()]
}
func (r *actionDefinition) DeletedFunction(key resources.ClusterObjectKey) DeletedFunction {
	return r.deleteders[key.GroupKind()]
}
func (r *actionDefinition) CommandFunction(cmd string) CommandFunction {
	if f := r.commands[cmd]; f != nil {
		return f
	}
	for _, m := range r.matchers {
		if m.Match(cmd) {
			return m.Command
		}
	}
	return nil
}

func (r *actionDefinition) Reconcile(logger logger.LogContext, obj resources.Object) reconcile.Status {
	if f := r.ReconcileFunction(obj); f != nil {
		return f(logger, obj)
	}
	return reconcile.Succeeded(logger)
}
func (r *actionDefinition) Delete(logger logger.LogContext, obj resources.Object) reconcile.Status {
	if f := r.DeleteFunction(obj); f != nil {
		return f(logger, obj)
	}
	return reconcile.Succeeded(logger)
}
func (r *actionDefinition) Deleted(logger logger.LogContext, key resources.ClusterObjectKey) reconcile.Status {
	if f := r.DeletedFunction(key); f != nil {
		return f(logger, key)
	}
	return reconcile.Succeeded(logger)
}

func (r *actionDefinition) Command(logger logger.LogContext, cmd string) reconcile.Status {
	if f := r.CommandFunction(cmd); f != nil {
		return f(logger, cmd)
	}
	return reconcile.Failed(logger, fmt.Errorf("unknown command %q", cmd))
}
