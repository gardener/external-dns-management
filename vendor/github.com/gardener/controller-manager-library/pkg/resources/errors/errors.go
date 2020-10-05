/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package errors

import "github.com/gardener/controller-manager-library/pkg/errors"

const (
	GROUP = "gardener/cml/resources"

	// formal

	// ERR_UNEXPECTED_TYPE is a general error for an unexpected type
	// objs:
	// - usage scenario
	// - object whose type is unexpected
	ERR_UNEXPECTED_TYPE = "UNEXPECTED_TYPE"

	// ERR_UNKNOWN is a generic error for an unknown element
	// objs:
	// - unknown element
	ERR_UNKNOWN = "UNKNOWN"

	// ERR_UNKNOWN_RESOURCE is an error for an unknown resource specification
	// objs:
	// - spec type
	// - unknown element
	ERR_UNKNOWN_RESOURCE = "UNKNOWN_RESOURCE"

	// ERR_UNEXPECTED_RESOURCE is an error for an unexpected resource given for a dedicated use case
	// objs:
	// - use case
	// - object
	ERR_UNEXPECTED_RESOURCE = "UNEXPECTED_RESOURCE"

	// ERR_FAILED is returned if operation failed for object
	// objs:
	// - operation
	// - object
	ERR_FAILED = "FAILED"

	// ERR_NAMESPACED is returned if resource is namespaced and requires namespace for identity
	// objs:
	// - element type info, i.e gvk
	ERR_NAMESPACED = "NAMESPACED"

	// ERR_NOT_NAMESPACED is returned if resource is not namespaced
	// objs:
	// - element type info, i.e gvk
	ERR_NOT_NAMESPACED = "NOT_NAMESPACED"

	// ERR_RESOURCE_MISMATCH is returned if resource object cannot handle instance of foreign resource
	// objs:
	// - called resource
	// - requested resource
	ERR_RESOURCE_MISMATCH = "RESOURCE_MISMATCH"

	// ERR_TYPE_MISMATCH is returned if wrong type given
	// objs:
	// - given object
	// - required type
	ERR_TYPE_MISMATCH = "TYPE_MISMATCH"

	// ERR_NO_STATUS_SUBRESOURCE is returned if resource has no status sub resource
	// objs:
	// - given resource spec
	ERR_NO_STATUS_SUBRESOURCE = "NO_STATUS_SUBRESOURCE"

	// informal

	ERR_OBJECT_REJECTED = "OBJECT_REJECTED"
	// objs: key

	ERR_NO_LIST_TYPE = "NO_LIST_TYPE"
	// objs: type with missing list type

	ERR_NON_UNIQUE_MAPPING = "NON_UNIQUE_MAPPING"
	// objs: key

	ERR_NOT_FOUND = "NOTFOUND"
	// objs: element type/kind, element spec

	ERR_INVALID = "INVALID"
	// objs: some invalid element

	ERR_INVALID_RESPONSE = "INVALID_RESPONSE"
	// objs:
	// - source
	// - response

	ERR_PERMISSION_DENIED = "PERMISSION_DENIED"
	// objs:
	// - source key
	// - relation
	// - used key

	ERR_CONFLICT = "CONFLICT"
	// objs:
	// - target
	// - reason

)

var (
	// ErrNotFound is returned if and element of a dedicated kind couldn't be found for a given name/spec
	ErrNotFound = errors.DeclareFormalType(GROUP, ERR_NOT_FOUND, "%s not found: %s")
	// ErrTypeMismatch is returned if wrong type given
	ErrTypeMismatch = errors.DeclareFormalType(GROUP, ERR_TYPE_MISMATCH, "unexpected type %T (expected %s)")
	// ErrUnexpectedType is returned if invalid type for a dedicated use case
	ErrUnexpectedType = errors.DeclareFormalType(GROUP, ERR_UNEXPECTED_TYPE, "unexpected type for %s: %T")
	// ErrUnknownResource is an error for an unknown resource specification
	ErrUnknownResource = errors.DeclareFormalType(GROUP, ERR_UNKNOWN_RESOURCE, "unknown resource for %s %q")
	// ErrUnexpectedResource is an error for an unexpected resource given for a dedicated use case
	ErrUnexpectedResource = errors.DeclareFormalType(GROUP, ERR_UNEXPECTED_RESOURCE, "unexpected resource for %s: %s")
	// ErrUnknown is a generic error for an unknown element
	ErrUnknown = errors.DeclareFormalType(GROUP, ERR_UNKNOWN, "unknown %s")
	// ErrFailed is returned if operation failed for object
	ErrFailed = errors.DeclareFormalType(GROUP, ERR_FAILED, "%s failed: %s")
	// ErrNamespaced is returned if resource is namespaced and requires namespace for identity
	ErrNamespaced = errors.DeclareFormalType(GROUP, ERR_NAMESPACED, "resource is namespaced: %s")
	// ErrNotNamespaced is returned if resource is not namespaced
	ErrNotNamespaced = errors.DeclareFormalType(GROUP, ERR_NOT_NAMESPACED, "resource is not namespaced: %s")
	// ErrResourceMismatch is returned if resource object cannot handle instance of foreign resource
	ErrResourceMismatch = errors.DeclareFormalType(GROUP, ERR_RESOURCE_MISMATCH, "resource object for %s cannot handle resource %s")
	// ErrNoStatusSubResource is returned if resource has no status sub resource
	ErrNoStatusSubResource = errors.DeclareFormalType(GROUP, ERR_NO_STATUS_SUBRESOURCE, "resource %q has no status sub resource")
)

func New(kind string, msgfmt string, args ...interface{}) error {
	return errors.Newf(GROUP, kind, args, msgfmt, args...)
}

func NewForObject(o interface{}, kind string, msgfmt string, args ...interface{}) error {
	return errors.Newf(GROUP, kind, []interface{}{o}, msgfmt, args...)
}

func NewForObjects(o []interface{}, kind string, msgfmt string, args ...interface{}) error {
	return errors.Newf(GROUP, kind, o, msgfmt, args...)
}

func Wrap(err error, kind string, msgfmt string, args ...interface{}) error {
	return errors.Wrapf(err, GROUP, kind, args, msgfmt, args...)
}

func WrapForObject(err error, o interface{}, kind string, msgfmt string, args ...interface{}) error {
	return errors.Wrapf(err, GROUP, kind, []interface{}{o}, msgfmt, args...)
}

func WrapForObjects(err error, o []interface{}, kind string, msgfmt string, args ...interface{}) error {
	return errors.Wrapf(err, GROUP, kind, o, msgfmt, args...)
}

func NewInvalid(msgfmt string, elem interface{}) error {
	return New(ERR_INVALID, msgfmt, elem)
}

func NewNotFound(msgfmt string, elem interface{}) error {
	return New(ERR_NOT_FOUND, msgfmt, elem)
}

func IsGroup(err error) bool {
	return errors.IsGroup(GROUP, err)
}

func IsKind(name string, err error) bool {
	return errors.IsKind(GROUP, name, err)
}
