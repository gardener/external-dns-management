/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package cluster

import "sync"

var elock sync.RWMutex

var extensions = []Extension{}

func RegisterExtension(e Extension) {
	elock.Lock()
	defer elock.Unlock()
	extensions = append(extensions, e)
}

func callExtensions(f func(e Extension) error) error {
	elock.RLock()
	defer elock.RUnlock()

	for _, e := range extensions {
		err := f(e)
		if err != nil {
			return err
		}
	}
	return nil
}
