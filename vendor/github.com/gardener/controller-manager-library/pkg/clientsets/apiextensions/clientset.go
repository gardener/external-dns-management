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

package apiextensions

import (
	"fmt"
	"github.com/gardener/controller-manager-library/pkg/clientsets"
	"k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	"k8s.io/client-go/rest"
)

var clientsetType = (*clientset.Interface)(nil)

func init() {
	clientsets.MustRegister(clientsetType, &factory{})
}

type factory struct{}

func (f *factory) Create(config *rest.Config) (interface{}, error) {
	return clientset.NewForConfig(config)
}

func Clientset(clientsets clientsets.Interface) (clientset.Interface, error) {
	cs, err := clientsets.Get(clientsetType)
	if err != nil {
		return nil, err
	}

	typedClientset, ok := cs.(clientset.Interface)
	if !ok {
		return nil, fmt.Errorf("unable to convert clientset to kubernetes clientset: %T", cs)
	}

	return typedClientset, nil
}
