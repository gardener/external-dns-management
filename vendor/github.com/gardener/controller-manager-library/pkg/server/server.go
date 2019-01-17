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
	"context"
	"fmt"
	"github.com/gardener/controller-manager-library/pkg/ctxutil"
	"net/http"
	"time"

	"github.com/gardener/controller-manager-library/pkg/logger"
)

func Register(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	logger.Infof("adding %s endpoint", pattern)
	http.HandleFunc(pattern, handler)
}

func RegisterHandler(pattern string, handler http.Handler) {
	logger.Infof("adding %s endpoint", pattern)
	http.Handle(pattern, handler)
}

// Serve starts a HTTP server.
func Serve(ctx context.Context, bindAddress string, port int) {
	logger.Info("starting http server")

	listenAddress := fmt.Sprintf("%s:%d", bindAddress, port)
	server := &http.Server{Addr: listenAddress, Handler: nil}

	go func() {
		<-ctx.Done()
		logger.Infof("shutting down server with timeout")
		ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
		server.Shutdown(ctx)
	}()

	go func() {
		logger.Infof("HTTP server started (serving on %s)", listenAddress)
		err := server.ListenAndServe()
		if err != nil {
			logger.Errorf("cannot start http server: %s", err)
		}
		logger.Infof("HTTP server stopped")
		ctxutil.Cancel(ctx)
	}()
}
