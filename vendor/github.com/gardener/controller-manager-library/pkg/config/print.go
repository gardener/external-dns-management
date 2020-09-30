/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package config

import "fmt"

type Writer func(msgfmt string, args ...interface{})

func PrintfWriter(msgfmt string, args ...interface{}) {
	fmt.Printf(msgfmt+"\n", args...)
}

func Print(log Writer, gap string, opts Options) {
	grp, ok := opts.(OptionSourceSource)
	if ok {
		log("%s* %s", gap, grp.Name())
		gap = gap + "  "
	}
	opts.VisitOptions(func(o *ArbitraryOption) bool {
		log("%s%s: %t: %v (%s)", gap, o.Name, o.Changed(), o.Value(), o.Description)
		return true
	})
	if ok {
		grp.VisitSources(func(key string, t OptionSource) bool {
			if sub, ok := t.(OptionGroup); ok {
				Print(log, gap, sub)
			} else {
				log("%s* (%s)", gap, key)
			}
			return true
		})
	}
}
