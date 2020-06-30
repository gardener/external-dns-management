/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 *
 */

package server

import (
	"net/http"

	"github.com/gardener/controller-manager-library/pkg/logger"
)

var servMux = http.NewServeMux()

func Register(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	logger.Infof("adding %s endpoint", pattern)
	servMux.HandleFunc(pattern, handler)
}

func RegisterHandler(pattern string, handler http.Handler) {
	logger.Infof("adding %s endpoint", pattern)
	servMux.Handle(pattern, handler)
}
