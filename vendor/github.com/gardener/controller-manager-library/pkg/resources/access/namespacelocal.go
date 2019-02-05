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
