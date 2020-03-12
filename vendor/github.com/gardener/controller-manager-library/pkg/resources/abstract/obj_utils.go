/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved.
 * This file is licensed under the Apache Software License, v. 2 except as noted
 * otherwise in the LICENSE file
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

package abstract

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (this *AbstractObject) GetLabel(name string) string {
	labels := this.ObjectData.GetLabels()
	if labels == nil {
		return ""
	}
	return labels[name]
}

func (this *AbstractObject) GetAnnotation(name string) string {
	annos := this.ObjectData.GetAnnotations()
	if annos == nil {
		return ""
	}
	return annos[name]
}

func (this *AbstractObject) GetOwnerReference() *metav1.OwnerReference {
	return metav1.NewControllerRef(this.ObjectData, this.GroupVersionKind())
}

func (this *AbstractObject) IsDeleting() bool {
	return this.GetDeletionTimestamp() != nil
}
