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

package healthz

import (
	"io"
	"net/http"

	"github.com/gardener/controller-manager-library/pkg/server"
)

func init() {
	server.Register("/healthz", Healthz)
}

// Healthz is a HTTP handler for the /healthz endpoint which responses with 200 OK status code
// if the Gardener controller manager is healthy; and with 500 Internal Server error status code otherwise.
func Healthz(w http.ResponseWriter, r *http.Request) {
	ok, info := HealthInfo()
	if ok {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusInternalServerError)
	}
	io.WriteString(w, info)
}
