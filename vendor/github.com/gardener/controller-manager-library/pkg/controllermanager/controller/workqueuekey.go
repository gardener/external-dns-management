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

package controller

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/runtime/schema"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/gardener/controller-manager-library/pkg/resources"
)

type Object interface {
	metav1.Object
	runtime.Object
}

type ObjectIdentity interface {
	GetGroupKind() schema.GroupKind
	GetName() string
	GetNamespace() string
}

func EncodeCommandKey(cmd string) string {
	return fmt.Sprintf("cmd:%s", cmd)
}

func EncodeObjectKeyForObject(o resources.Object) string {
	return EncodeObjectKey(o.GetCluster().GetName(), o.Key())
}

func EncodeObjectKey(cluster string, o resources.ObjectKey) string {
	return fmt.Sprintf("obj:%s:%s", cluster, EncodeObjectSubKey(o))
}

func EncodeObjectSubKey(o resources.ObjectKey) string {
	gk := o.GroupKind()
	return fmt.Sprintf("%s/%s/%s/%s", gk.Group, gk.Kind, o.Namespace(), o.Name())
}

func DecodeObjectSubKey(key string) (apiGroup, kind, namespace, name string, err error) {
	parts := strings.Split(key, "/")
	switch len(parts) {
	case 1:
		// name only, no namespace
		return "", "", "", parts[0], nil
	case 2:
		// kind, name
		return "", parts[0], "", parts[1], nil
	case 3:
		// kind, namespace and name
		return "", parts[0], parts[1], parts[2], nil
	case 4:
		// apiGroup, kind, namespace and name
		return parts[0], parts[1], parts[2], parts[3], nil
	}

	return "", "", "", "", fmt.Errorf("unexpected key format: %q", key)
}
