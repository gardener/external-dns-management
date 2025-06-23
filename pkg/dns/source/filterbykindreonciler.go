// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package source

import (
	"fmt"
	"strings"

	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller"
	"github.com/gardener/controller-manager-library/pkg/controllermanager/controller/reconcile"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
)

func reconcilerTypeFilterByKind(kind string, reconcilerType controller.ReconcilerType) controller.ReconcilerType {
	return func(c controller.Interface) (reconcile.Interface, error) {
		reconciler, err := reconcilerType(c)
		if err != nil {
			return nil, err
		}
		return &filterByKindReconciler{
			nested:        reconciler,
			kindSubstring: fmt.Sprintf("-%s-", strings.ToLower(kind)),
		}, nil
	}
}

// filterByKindReconciler is a reconciler that filters out entries by kind.
type filterByKindReconciler struct {
	nested        reconcile.Interface
	kindSubstring string
}

var _ reconcile.Interface = &filterByKindReconciler{}
var _ reconcile.StartInterface = &filterByKindReconciler{}
var _ reconcile.SetupInterface = &filterByKindReconciler{}
var _ reconcile.CleanupInterface = &filterByKindReconciler{}

func (f filterByKindReconciler) Start() error {
	if itf, ok := f.nested.(reconcile.StartInterface); ok {
		return itf.Start()
	}
	return nil
}

func (f filterByKindReconciler) Setup() error {
	if itf, ok := f.nested.(reconcile.SetupInterface); ok {
		return itf.Setup()
	}
	return nil
}

func (f filterByKindReconciler) Cleanup() error {
	if itf, ok := f.nested.(reconcile.CleanupInterface); ok {
		return itf.Cleanup()
	}
	return nil
}

func (f filterByKindReconciler) Reconcile(logger logger.LogContext, object resources.Object) reconcile.Status {
	// Check if the object is relevant by its name. GenerateName is used as the object name may be shortened for long names.
	if !f.isRelevantByHeuristicGenerateName(object.GetGenerateName(), object.GetName()) {
		return reconcile.Succeeded(logger)
	}
	return f.nested.Reconcile(logger, object)
}

func (f filterByKindReconciler) Delete(logger logger.LogContext, object resources.Object) reconcile.Status {
	// Check if the object is relevant by its name. GenerateName is used as the object name may be shortened for long names.
	if !f.isRelevantByHeuristicGenerateName(object.GetGenerateName(), object.GetName()) {
		return reconcile.Succeeded(logger)
	}
	return f.nested.Delete(logger, object)
}

func (f filterByKindReconciler) Deleted(logger logger.LogContext, key resources.ClusterObjectKey) reconcile.Status {
	if !f.isRelevantByHeuristic(key.Name()) {
		return reconcile.Succeeded(logger)
	}
	return f.nested.Deleted(logger, key)
}

func (f filterByKindReconciler) Command(logger logger.LogContext, cmd string) reconcile.Status {
	return f.nested.Command(logger, cmd)
}

// isRelevantByHeuristic returns true if the entry name contains the kind substring.
// This is a heuristic which relies on the fact that the SourceReconciler adds the kind to the entry name.
func (this *filterByKindReconciler) isRelevantByHeuristic(objectName string) bool {
	if len(objectName) >= 63 {
		// The object name is too long. The kind substring cannot be checked, as it may be shortened.
		return true
	}
	return strings.Contains(objectName, this.kindSubstring)
}

// isRelevantByHeuristicGenerateName returns true if the entry generate name contains the kind substring.
// This is a heuristic which relies on the fact that the SourceReconciler adds the kind to the entry name.
func (this *filterByKindReconciler) isRelevantByHeuristicGenerateName(generateName, name string) bool {
	if generateName == "" {
		// The generateName may be empty for very old entries. Fallback to old heuristic in this case.
		return this.isRelevantByHeuristic(name)
	}
	return strings.HasSuffix(generateName, this.kindSubstring)
}
