/*
 * Copyright 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package embed

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"log"
	"net"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/external-dns-management/pkg/server/remote/common"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type RemoteAccessServerConfig struct {
	Port               int
	CACertFilename     string
	ServerCertFilename string
	ServerKeyFilename  string
}

type CreateServerFunc func(logctx logger.LogContext) common.RemoteProviderServer

var serverFunc CreateServerFunc

func RegisterCreateServerFunc(f CreateServerFunc) {
	serverFunc = f
}

func loadTLSCredentials(cfg *RemoteAccessServerConfig) (credentials.TransportCredentials, error) {
	// Load certificate of the CA who signed client's certificate
	pemClientCA, err := ioutil.ReadFile(cfg.CACertFilename)
	if err != nil {
		return nil, err
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(pemClientCA) {
		return nil, fmt.Errorf("failed to add client CA's certificate")
	}

	// Load server's certificate and private key
	serverCert, err := tls.LoadX509KeyPair(cfg.ServerCertFilename, cfg.ServerKeyFilename)
	if err != nil {
		return nil, err
	}

	config := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    certPool,
	}

	return credentials.NewTLS(config), nil
}

func StartDNSHandlerServer(logctx logger.LogContext, config *RemoteAccessServerConfig) (common.RemoteProviderServer, error) {
	creds, err := loadTLSCredentials(config)
	if err != nil {
		return nil, err
	}
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", config.Port))
	if err != nil {
		return nil, err
	}
	s := grpc.NewServer(grpc.Creds(creds))
	logctx = logctx.NewContext("server", "remoteaccess")
	server := serverFunc(logctx)
	common.RegisterRemoteProviderServer(s, server)
	logctx.Infof("DNSHandler server listening at %v", lis.Addr())
	go func() {
		if err := s.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()
	return server, nil
}
