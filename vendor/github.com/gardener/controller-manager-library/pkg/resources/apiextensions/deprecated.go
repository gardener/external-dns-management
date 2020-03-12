/*
 * Copyright 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *       http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 *
 */

package apiextensions

import (
	"k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
)

////////////////////////////////////////////////////////////////////////////////

func CreateCRDObjectWithStatus(groupName, version, rkind, rplural, shortName string, namespaces bool, columns ...v1beta1.CustomResourceColumnDefinition) *v1beta1.CustomResourceDefinition {
	return _CreateCRDObject(true, groupName, version, rkind, rplural, shortName, namespaces, columns...)
}

func CreateCRDObject(groupName, version, rkind, rplural, shortName string, namespaces bool, columns ...v1beta1.CustomResourceColumnDefinition) *v1beta1.CustomResourceDefinition {
	return _CreateCRDObject(false, groupName, version, rkind, rplural, shortName, namespaces, columns...)
}

func _CreateCRDObject(status bool, groupName, version, rkind, rplural, shortName string, namespaces bool, columns ...v1beta1.CustomResourceColumnDefinition) *v1beta1.CustomResourceDefinition {
	crdName := rplural + "." + groupName
	scope := v1beta1.ClusterScoped
	if namespaces {
		scope = v1beta1.NamespaceScoped
	}
	crd := &v1beta1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: crdName,
		},
		Spec: v1beta1.CustomResourceDefinitionSpec{
			Group:   groupName,
			Version: version,
			Scope:   scope,
			Names: v1beta1.CustomResourceDefinitionNames{
				Plural: rplural,
				Kind:   rkind,
			},
		},
	}

	if status {
		crd.Spec.Subresources = &v1beta1.CustomResourceSubresources{Status: &v1beta1.CustomResourceSubresourceStatus{}}
	}
	for _, c := range columns {
		crd.Spec.AdditionalPrinterColumns = append(crd.Spec.AdditionalPrinterColumns, c)
	}
	crd.Spec.AdditionalPrinterColumns = append(crd.Spec.AdditionalPrinterColumns, v1beta1.CustomResourceColumnDefinition{Name: "AGE", Type: "date", JSONPath: ".metadata.creationTimestamp"})

	if len(shortName) != 0 {
		crd.Spec.Names.ShortNames = []string{shortName}
	}

	return crd
}

func CreateCRD(cluster resources.Cluster, groupName, version, rkind, rplural, shortName string, namespaces bool, columns ...v1beta1.CustomResourceColumnDefinition) error {
	crd := CreateCRDObject(groupName, version, rkind, rplural, shortName, namespaces, columns...)
	return CreateCRDFromObject(logger.New(), cluster, crd, "")
}
