/*
 * SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 */

package fieldpath

type Path interface {
	Get(obj interface{}) (interface{}, error)
	Set(obj interface{}, value interface{}) error
}
