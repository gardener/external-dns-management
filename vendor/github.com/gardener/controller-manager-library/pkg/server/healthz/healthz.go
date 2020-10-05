/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
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
