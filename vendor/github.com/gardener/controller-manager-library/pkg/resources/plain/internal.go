/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package plain

import (
	"k8s.io/apimachinery/pkg/runtime"
)

type Internal interface {
	Interface

	CreateData(name ...ObjectDataName) ObjectData
	CreateListData() runtime.Object
	CheckOType(obj ObjectData, unstructured ...bool) error
}
