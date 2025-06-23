// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dns

type GroupKey interface {
	String() string
}

type GroupInfo interface{}

type Group struct {
	Key  GroupKey
	Info GroupInfo
	Sets map[string]*DNSSet
}

type Cache struct {
	Groups map[GroupKey]Group
}
