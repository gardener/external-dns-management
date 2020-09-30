/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package configmain

import "sync"

type Extension func(cfg *Config)

var lock sync.Mutex

var extensions = []Extension{}

func RegisterExtension(e Extension) {
	lock.Lock()
	defer lock.Unlock()

	extensions = append(extensions, e)
}

func addExtensions(cfg *Config) {
	lock.Lock()
	defer lock.Unlock()

	for _, e := range extensions {
		e(cfg)
	}
}
