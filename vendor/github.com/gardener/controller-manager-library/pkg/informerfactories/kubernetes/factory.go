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

package kubernetes

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/clientsets"
	typed "github.com/gardener/controller-manager-library/pkg/clientsets/kubernetes"
	"github.com/gardener/controller-manager-library/pkg/informerfactories"
	"k8s.io/client-go/informers"
)

var informerKey = (*informers.SharedInformerFactory)(nil)

func init() {
	informerfactories.MustRegister(informerKey, &factory{})
}

type factory struct{}

func (f *factory) Create(clientsets clientsets.Interface) (interface{}, error) {
	clientset, err := typed.Clientset(clientsets)
	if err != nil {
		return nil, err
	}

	return informers.NewSharedInformerFactory(clientset, 0), nil
}

func SharedInformerFactory(informerfactories informerfactories.Interface) (informers.SharedInformerFactory, error) {
	factory, err := informerfactories.Get(informerKey)
	if err != nil {
		return nil, err
	}

	typedFactory, ok := factory.(informers.SharedInformerFactory)
	if !ok {
		return nil, fmt.Errorf("unable to convert factory to kubernetes factory: %T", factory)
	}

	return typedFactory, nil
}
