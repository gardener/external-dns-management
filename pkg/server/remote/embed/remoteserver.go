// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package embed

import (
	"context"
	"crypto/x509"
	"fmt"
	"log"
	"net"
	"os"
	"runtime/debug"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/controller-manager-library/pkg/resources"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"

	"github.com/gardener/external-dns-management/pkg/server/remote/common"
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

type CreateServerFunc func(logctx logger.LogContext) (common.RemoteProviderServer, error)

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
	s := grpc.NewServer(grpc.Creds(creds), grpc.UnaryInterceptor(recoveryUnaryInterceptor(logctx)))
	server, err := serverFunc(logctx)
	if err != nil {
		return nil, err
	}
	common.RegisterRemoteProviderServer(s, server)
	logctx.Infof("DNSHandler server listening at %v", lis.Addr())
	go func() {
		if err := s.Serve(lis); err != nil {
			log.Fatalf("failed to serve: %v", err)
		}
	}()
	return server, nil
}

// recoveryUnaryInterceptor turns a panic in a handler into an Internal gRPC error
// instead of letting it crash the whole process.
func recoveryUnaryInterceptor(logctx logger.LogContext) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp any, err error) {
		defer func() {
			if r := recover(); r != nil {
				logctx.Errorf("panic in %s: %v\n%s", info.FullMethod, r, debug.Stack())
				err = status.Errorf(codes.Internal, "internal error")
			}
		}()
		return handler(ctx, req)
	}
}
