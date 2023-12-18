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
	"errors"
	"sync"

	"github.com/gardener/controller-manager-library/pkg/resources"
)

type (
	Elements []resources.Object
	Executor func(resources.Object) error
)

func ProcessElements(elems Elements, exec Executor, processors int) error {
	ch := make(chan resources.Object, processors)
	wg := sync.WaitGroup{}

	errs := &synchedErrs{}
	for i := 1; i <= processors; i++ {
		wg.Add(1)
		go func() {
			for {
				e, ok := <-ch
				if !ok {
					break
				}
				if err := exec(e); err != nil {
					errs.add(err)
				}
			}
			wg.Done()
		}()
	}
	for _, e := range elems {
		ch <- e
	}
	close(ch)
	wg.Wait()

	return errs.join()
}

type synchedErrs struct {
	sync.Mutex
	errs []error
}

func (e *synchedErrs) add(err error) {
	if err == nil {
		return
	}
	e.Lock()
	defer e.Unlock()
	e.errs = append(e.errs, err)
}

func (e *synchedErrs) join() error {
	e.Lock()
	defer e.Unlock()
	if e.errs == nil {
		return nil
	}
	return errors.Join(e.errs...)
}
