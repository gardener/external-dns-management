/*
 * SPDX-FileCopyrightText: 2019 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
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
