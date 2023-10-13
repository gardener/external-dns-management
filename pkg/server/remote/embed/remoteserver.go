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
	"crypto/x509"
	"fmt"
	"log"
	"net"
	"os"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"github.com/gardener/external-dns-management/pkg/server/remote/common"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	corev1 "k8s.io/api/core/v1"
)

// ServerSecretUpdateHandler is called on updated server secret
type ServerSecretUpdateHandler func(secret *corev1.Secret)

// ServerSecretProvider informs handlers on updated secret
type ServerSecretProvider interface {
	UpdateSecret(secret *corev1.Secret)
	AddUpdateHandler(handler ServerSecretUpdateHandler)
}

type RemoteAccessServerConfig struct {
	Port                 int
	CACertFilename       string
	SecretName           resources.ObjectName
	ServerSecretProvider ServerSecretProvider
}

type CreateServerFunc func(logctx logger.LogContext) common.RemoteProviderServer

var serverFunc CreateServerFunc

func RegisterCreateServerFunc(f CreateServerFunc) {
	serverFunc = f
}

func loadTLSCredentials(logctx logger.LogContext, cfg *RemoteAccessServerConfig) (credentials.TransportCredentials, error) {
	// Load certificate of the CA who signed client's certificate
	pemClientCA, err := os.ReadFile(cfg.CACertFilename)
	if err != nil {
		return nil, err
	}

	certPool := x509.NewCertPool()
	if !certPool.AppendCertsFromPEM(pemClientCA) {
		return nil, fmt.Errorf("failed to add client CA's certificate")
	}

	return newDynamicTransportCredentials(logctx, certPool, cfg.ServerSecretProvider), nil
}

func StartDNSHandlerServer(logctx logger.LogContext, config *RemoteAccessServerConfig) (common.RemoteProviderServer, error) {
	logctx = logctx.NewContext("server", "remoteaccess")
	creds, err := loadTLSCredentials(logctx, config)
	if err != nil {
		return nil, err
	}
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", config.Port))
	if err != nil {
		return nil, err
	}
	s := grpc.NewServer(grpc.Creds(creds))
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
