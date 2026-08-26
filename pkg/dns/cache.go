// SPDX-FileCopyrightText: Copyright Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package dns

type GroupKey interface {
	String() string
}

type GroupInfo any

type Group struct {
	Key  GroupKey
	Info GroupInfo
	Sets map[string]*DNSSet
}

type Cache struct {
	Groups map[GroupKey]Group
}
