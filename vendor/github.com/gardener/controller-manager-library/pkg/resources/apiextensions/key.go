/*
 * SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 */

package apiextensions

import (
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/gardener/controller-manager-library/pkg/resources"
)

func NewKey(crdName string) resources.ObjectKey {
	return resources.NewKey(schema.GroupKind{apiextensions.GroupName, "CustomResourceDefinition"}, "", crdName)
}
