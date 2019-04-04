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

package utils

import (
	"github.com/gardener/controller-manager-library/pkg/resources"
	"sync"
)

type Elements []resources.Object
type Executor func(resources.Object)

func ProcessElements(elems Elements, exec Executor, processors int) {
	ch := make(chan resources.Object, processors)
	wg := sync.WaitGroup{}

	for i := 1; i <= processors; i++ {
		wg.Add(1)
		go func() {
			for {
				e, ok := <-ch
				if !ok {
					break
				}
				exec(e)
			}
			wg.Done()
		}()
	}
	for _, e := range elems {
		ch <- e
	}
	close(ch)
	wg.Wait()
}
