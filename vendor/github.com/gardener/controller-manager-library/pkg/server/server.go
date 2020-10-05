/*
 * SPDX-FileCopyrightText: 2020 SAP SE or an SAP affiliate company and Gardener contributors
 *
 * SPDX-License-Identifier: Apache-2.0
 */

package server

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gardener/controller-manager-library/pkg/certs"
	"github.com/gardener/controller-manager-library/pkg/ctxutil"
	"github.com/gardener/controller-manager-library/pkg/logger"
)

type HTTPServer struct {
	name    string
	servMux *http.ServeMux
	ctx     context.Context

	logger.LogContext
}

func NewHTTPServer(ctx context.Context, logger logger.LogContext, name string) *HTTPServer {
	this := &HTTPServer{name: name, ctx: ctx, LogContext: logger, servMux: http.NewServeMux()}
	return this
}

func NewDefaultHTTPServer(ctx context.Context, logger logger.LogContext, name string) *HTTPServer {
	this := &HTTPServer{name: name, ctx: ctx, LogContext: logger, servMux: servMux}
	return this
}

func (this *HTTPServer) Register(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	pattern = NormPath(pattern)
	this.Infof("adding %s endpoint: %s", this.name, pattern)
	this.servMux.HandleFunc(pattern, handler)
}

func (this *HTTPServer) RegisterHandler(pattern string, handler http.Handler) {
	pattern = NormPath(pattern)
	this.Infof("adding %s endpoint: %s", this.name, pattern)
	this.servMux.Handle(pattern, handler)
}

// Start starts an HTTP/S server.
func (this *HTTPServer) Start(source certs.CertificateSource, bindAddress string, port int) {
	var tlscfg *tls.Config

	listenAddress := fmt.Sprintf("%s:%d", bindAddress, port)
	if source != nil {
		this.Infof("starting %s as https server (serving on %s)", this.name, listenAddress)
		tlscfg = &tls.Config{
			NextProtos:     []string{"h2"},
			GetCertificate: source.GetCertificate,
		}
	} else {
		this.Infof("starting %s as http server (serving on %s)", this.name, listenAddress)
	}
	server := &http.Server{
		Addr:      listenAddress,
		Handler:   this.servMux,
		TLSConfig: tlscfg,
	}

	ctxutil.WaitGroupAdd(this.ctx)
	go func() {
		<-this.ctx.Done()
		this.Infof("shutting down server %q with timeout", this.name)
		ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)
		server.Shutdown(ctx)
	}()

	go func() {
		var err error
		this.Infof("server %q started", this.name)
		if tlscfg != nil {
			err = server.ListenAndServeTLS("", "")
		} else {
			err = server.ListenAndServe()
		}
		if err != nil && err != http.ErrServerClosed {
			logger.Errorf("cannot start server %q: %s", this.name, err)
		}
		this.Infof("server %q stopped", this.name)
		ctxutil.Cancel(this.ctx)
		ctxutil.WaitGroupDone(this.ctx)
	}()
}

func NormPath(p string) string {
	if !strings.HasPrefix(p, "/") {
		return "/" + p
	}
	return p
}
