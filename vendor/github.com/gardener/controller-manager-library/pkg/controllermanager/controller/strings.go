/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package controller

import (
	"fmt"
	"time"
)

func toString(o interface{}) string {
	s := "["
	sep := ""

	switch v := o.(type) {
	case map[string]ReconcilerType:
		for n, r := range v {
			s = fmt.Sprintf("%s%s%s: %T", s, sep, n, r)
			sep = ", "
		}
	case map[string]PoolDefinition:
		for _, p := range v {
			s = fmt.Sprintf("%s%s%s", s, sep, toString(p))
			sep = ", "
		}
	case PoolDefinition:
		return fmt.Sprintf("%s (size %d, period %d sec)", v.GetName(), v.Size(), v.Period()/time.Second)
	case Watches:
		for n, w := range v {
			s = fmt.Sprintf("%s%s%s: %s", s, sep, n, toString(w))
			sep = ", "
		}
	case []Watch:
		for _, w := range v {
			s = fmt.Sprintf("%s%s%s", s, sep, toString(w))
			sep = ", "
		}
	case Watch:
		return fmt.Sprintf("%s in %s with %s", v.ResourceType(), v.PoolName(), v.Reconciler())

	case Commands:
		for n, c := range v {
			s = fmt.Sprintf("%s%s%s: %s", s, sep, n, toString(c))
			sep = ", "
		}
	case []Command:
		for _, c := range v {
			s = fmt.Sprintf("%s%s%s", s, sep, toString(c))
			sep = ", "
		}
	case Command:
		return fmt.Sprintf("%s in %s with %s", v.Key(), v.PoolName(), v.Reconciler())
	default:
		return fmt.Sprintf("%s", o)
	}
	return s + "]"
}
