/*
 * Copyright 2020 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *       http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
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
