/*
 * SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 */

package utils

type StringStack []string

func (this *StringStack) Push(name string) {
	if this != nil {
		*this = append(*this, name)
	}
}

func (this *StringStack) Pop() string {
	if this != nil && len(*this) > 0 {
		result := (*this)[0]
		*this = (*this)[1:]
		return result
	}
	return ""
}

func (this StringStack) Peek() string {
	if len(this) > 0 {
		return this[0]
	}
	return ""
}

func (this StringStack) Size() int {
	return len(this)
}

func (this StringStack) Empty() bool {
	return len(this) == 0
}
