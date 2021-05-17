/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 */

package watches

import (
	"fmt"

	"github.com/gardener/controller-manager-library/pkg/config"
)

func getStringOptionValue(wctx WatchContext, name string, srcnames ...string) string {
	for _, sn := range srcnames {
		src, err := wctx.GetOptionSource(sn)
		if err != nil {
			panic(fmt.Errorf("option source %q not found for option selection in controller resource for %s: %s",
				src, wctx.Name(), err))
		}
		if opts, ok := src.(config.Options); ok {
			opt := opts.GetOption(name)
			if opt != nil {
				return opt.StringValue()
			}
		} else {
			panic(fmt.Errorf("option source %q for option selection in controller resource for %s has no option access: %s",
				src, wctx.Name(), err))
		}
	}
	value, err := wctx.GetStringOption(name)
	if err != nil {
		panic(fmt.Errorf("option %q not found for option selection in controller resource for %s: %s",
			name, wctx.Name(), err))
	}
	return value
}
