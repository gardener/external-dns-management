// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package embed

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net"
	"reflect"
	"sync/atomic"
	"time"

	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/external-dns-management/pkg/server/metrics"
	atomic2 "go.uber.org/atomic"
	"google.golang.org/grpc/credentials"
	corev1 "k8s.io/api/core/v1"
)

type dynamicTransportCredentials struct {
	logctx               logger.LogContext
	certPool             *x509.CertPool
	serverSecretProvider ServerSecretProvider
	oldCertificates      atomic.Value // value type: []tls.Certificate
	currentTLS           atomic.Value // value type: credentials.TransportCredentials
	lastResourceVersion  atomic2.String
}

var _ credentials.TransportCredentials = &dynamicTransportCredentials{}

func newDynamicTransportCredentials(logctx logger.LogContext, certPool *x509.CertPool, provider ServerSecretProvider) credentials.TransportCredentials {
	dyn := &dynamicTransportCredentials{
		logctx:               logctx.NewContext("tc", "transport-credentials"),
		certPool:             certPool,
		serverSecretProvider: provider,
	}
	dyn.oldCertificates.Store([]tls.Certificate{})
	dyn.updateTLS(nil)
	provider.AddUpdateHandler(dyn.updateTLS)
	return dyn
}

func (d *dynamicTransportCredentials) current() credentials.TransportCredentials {
	return d.currentTLS.Load().(credentials.TransportCredentials)
}

func (d *dynamicTransportCredentials) updateTLS(secret *corev1.Secret) {
	resourceVersion := ""
	if secret != nil {
		resourceVersion = secret.ResourceVersion
	}
	initial := d.currentTLS.Load() == nil
	if initial || d.lastResourceVersion.Load() != resourceVersion {
		d.lastResourceVersion.Store(resourceVersion)
		tls, ok := d.createTLS(secret)
		if initial || ok {
			d.currentTLS.Store(tls)
		}
		d.logctx.Infof("new credentials from secret: version %q (%t)", resourceVersion, ok)
	}
}

func (d *dynamicTransportCredentials) createTLS(secret *corev1.Secret) (credentials.TransportCredentials, bool) {
	config := &tls.Config{
		MinVersion:   tls.VersionTLS12,
		Certificates: []tls.Certificate{},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    d.certPool,
	}

	ok := false
	oldCredentials := d.oldCertificates.Load().([]tls.Certificate)
	if secret != nil && secret.Data != nil {
		certPEMBlock := secret.Data[corev1.TLSCertKey]
		keyPEMBlock := secret.Data[corev1.TLSPrivateKeyKey]
		cert, err := tls.X509KeyPair(certPEMBlock, keyPEMBlock)
		if err == nil {
			ok = true
			config.Certificates = append(config.Certificates, cert)
			config.Certificates = appendValidCertificates(config.Certificates, oldCredentials)
		}
	}
	if !ok {
		config.Certificates = appendValidCertificates(config.Certificates, oldCredentials)
	}
	metrics.ReportRemoteAccessCertificates(len(config.Certificates))
	old := append([]tls.Certificate{}, config.Certificates...)
	d.oldCertificates.Store(old)
	return credentials.NewTLS(config), ok
}

func appendValidCertificates(certs []tls.Certificate, oldCerts []tls.Certificate) []tls.Certificate {
outer:
	for _, tlscert := range oldCerts {
		for _, cert := range certs {
			if reflect.DeepEqual(cert.Certificate, tlscert.Certificate) {
				continue outer
			}
		}
		c, err := x509.ParseCertificate(tlscert.Certificate[0])
		if err == nil && c.NotAfter.After(time.Now()) {
			certs = append(certs, tlscert)
		}
	}
	return certs
}

func (d *dynamicTransportCredentials) ClientHandshake(ctx context.Context, s string, conn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	return d.current().ClientHandshake(ctx, s, conn)
}

func (d *dynamicTransportCredentials) ServerHandshake(conn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	return d.current().ServerHandshake(conn)
}

func (d *dynamicTransportCredentials) Info() credentials.ProtocolInfo {
	return d.current().Info()
}

func (d *dynamicTransportCredentials) Clone() credentials.TransportCredentials {
	return d.current().Clone()
}

func (d *dynamicTransportCredentials) OverrideServerName(s string) error {
	return d.current().OverrideServerName(s)
}
