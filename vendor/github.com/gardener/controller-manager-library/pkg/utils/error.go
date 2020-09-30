/*
 * SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 */

package utils

// Must panics on non-nil errors.  Useful to handling programmer level errors.
func Must(args ...interface{}) {
	if len(args) > 0 {
		if !IsNil(args[len(args)-1]) {
			panic(args[len(args)-1])
		}
	}
}

func Error(args ...interface{}) error {
	if len(args) == 0 {
		return nil
	}
	if err, ok := args[len(args)-1].(error); ok {
		return err
	}
	return nil
}

func FirstValue(args ...interface{}) interface{} {
	if len(args) == 0 {
		return nil
	}
	return args[0]
}
