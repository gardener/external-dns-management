/*
 * SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 */

package ctxutil

// contextKey is a value for use with context.WithValue. It's used as
// a pointer so it fits in an interface{} without allocation.
type contextKey struct {
	name string
}

// Key is a basic interface for keys used to identify
// elements for various purposes. It just provides
// String() method to produce a human readable representation.
// A Key MUST always be go comparable to be usable as key
// and ist MUST always be immutable, because it will be used
// a key field in maps.
// The key object itself will be used as key, not the string representation.
// Therefore the key must be unique, but not the string representation
// A typical generic implementation is given by the SimpleKey function
// that provides a simple key unique for a go program with every call.
type Key interface {
	String() string
}

func (k *contextKey) String() string { return k.name }

// SimpleKey provides a unique key in the scope of a
// go program for every call.
// It uses the address of an object as key identity.
func SimpleKey(name string) Key {
	return &contextKey{name}
}
