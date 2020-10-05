/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package access

import (
	"github.com/gardener/controller-manager-library/pkg/resources"
)

type NamespaceLocalAccessOnly struct {
}

var _ AccessController = &NamespaceLocalAccessOnly{}

func (this *NamespaceLocalAccessOnly) Name() string {
	return "LocalNamespaceAccessOnly"
}

func (this *NamespaceLocalAccessOnly) Allowed(src resources.ClusterObjectKey, verb string, tgt resources.ClusterObjectKey) (int, error) {
	if src.Cluster() != tgt.Cluster() {
		return ACCESS_DENIED, nil
	}
	if src.Namespace() != tgt.Namespace() {
		return ACCESS_DENIED, nil
	}
	return ACCESS_GRANTED, nil
}

func RegisterNamespaceOnlyAccess() {
	Register(&NamespaceLocalAccessOnly{}, "", MIN_PRIO)
}
