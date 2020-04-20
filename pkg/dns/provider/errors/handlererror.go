/*
 * Copyright 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 *
 */

package errors

import (
	pkgerrors "github.com/pkg/errors"
)

type handlerError struct {
	err error
}

func WrapAsHandlerError(err error, msg string) error {
	return pkgerrors.Wrap(&handlerError{err: err}, msg)
}

func WrapfAsHandlerError(err error, msg string, args ...interface{}) error {
	return pkgerrors.Wrapf(&handlerError{err: err}, msg, args...)
}

func IsHandlerError(err error) bool {
	_, ok := err.(*handlerError)
	return ok
}

func (e *handlerError) Error() string {
	return e.err.Error()
}

func (e *handlerError) Cause() error {
	return e.err
}
