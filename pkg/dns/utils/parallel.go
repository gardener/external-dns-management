// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

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
