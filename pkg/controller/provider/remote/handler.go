/*
 * Copyright 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package remote

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/route53"
	"github.com/gardener/controller-manager-library/pkg/logger"
	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
	"github.com/gardener/external-dns-management/pkg/server/remote/common"
	"github.com/gardener/external-dns-management/pkg/server/remote/conversion"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
)

type Handler struct {
	provider.DefaultDNSHandler
	config          provider.DNSHandlerConfig
	cache           provider.ZoneCache
	clientID        string
	remoteNamespace string
	currentToken    string
	connection      *grpc.ClientConn
	client          common.RemoteProviderClient
	sess            *session.Session
	r53             *route53.Route53
}

var _ provider.DNSHandler = &Handler{}

func NewHandler(c *provider.DNSHandlerConfig) (provider.DNSHandler, error) {
	advancedConfig := c.Options.AdvancedOptions.GetAdvancedConfig()
	c.Logger.Infof("advanced options: %s", advancedConfig)

	h := &Handler{
		DefaultDNSHandler: provider.NewDefaultDNSHandler(TYPE_CODE),
		config:            *c,
		clientID:          getClientID(),
	}

	serverEndpoint, err := c.GetRequiredProperty("REMOTE_ENDPOINT", "remoteEndpoint")
	if err != nil {
		return nil, err
	}
	serverCA_PEM := c.GetDefaultedProperty("SERVER_CA_CERT", "", "ca.crt")
	clientCert_PEM, err := c.GetRequiredProperty("CLIENT_CERT", corev1.TLSCertKey)
	if err != nil {
		return nil, err
	}
	clientKey_PEM, err := c.GetRequiredProperty("CLIENT_KEY", corev1.TLSPrivateKeyKey)
	if err != nil {
		return nil, err
	}
	h.remoteNamespace, err = c.GetRequiredProperty("NAMESPACE", "namespace")
	if err != nil {
		return nil, err
	}
	overrideServerName := c.GetDefaultedProperty("OVERRIDE_SERVER_NAME", "", "overrideServerName")
	c.Logger.Infof("creating remote handler for %s, namespace: %s, overrideServerName: %s", serverEndpoint, h.remoteNamespace, overrideServerName)

	creds, err := h.loadTLSCredentials([]byte(serverCA_PEM), []byte(clientCert_PEM), []byte(clientKey_PEM))
	if err != nil {
		return nil, err
	}
	if overrideServerName != "" {
		err = creds.OverrideServerName(overrideServerName)
		if err != nil {
			return nil, err
		}
	}

	h.connection, err = grpc.Dial(serverEndpoint, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, err
	}
	h.client = common.NewRemoteProviderClient(h.connection)

	h.cache, err = provider.NewZoneCache(c.CacheConfig, c.Metrics, nil, h.getZones, h.getZoneState)
	if err != nil {
		return nil, err
	}

	return h, nil
}

func getClientID() string {
	if provider.RemoteAccessClientID != "" {
		return provider.RemoteAccessClientID
	}
	if hostname := os.Getenv("HOSTNAME"); hostname != "" {
		return hostname
	}
	if hostname, err := os.Hostname(); err == nil && hostname != "" {
		return hostname
	}
	return fmt.Sprintf("pid-%d", os.Getpid())
}

// serverCA_PEM: certificate (PEM) of the CA who signed server's certificate
func (h *Handler) loadTLSCredentials(serverCA_PEM, clientCert_PEM, clientKey_PEM []byte) (credentials.TransportCredentials, error) {
	// Load client's certificate and private key
	clientCert, err := tls.X509KeyPair(clientCert_PEM, clientKey_PEM)
	if err != nil {
		return nil, err
	}

	// Create the credentials and return it
	config := &tls.Config{
		Certificates: []tls.Certificate{clientCert},
	}

	if len(serverCA_PEM) > 0 {
		certPool := x509.NewCertPool()
		if !certPool.AppendCertsFromPEM(serverCA_PEM) {
			return nil, fmt.Errorf("failed to add server CA's certificate")
		}
		config.RootCAs = certPool
	}

	return credentials.NewTLS(config), nil
}

func (h *Handler) Release() {
	h.cache.Release()
	if h.connection != nil {
		h.connection.Close()
	}
}

func (h *Handler) GetZones() (provider.DNSHostedZones, error) {
	return h.cache.GetZones()
}

func (h *Handler) login(ctx context.Context) error {
	h.config.RateLimiter.Accept()
	response, err := h.client.Login(ctx, &common.LoginRequest{
		Namespace: h.remoteNamespace,
		CliendID:  h.clientID,
	})
	if err != nil {
		if s, ok := status.FromError(err); ok {
			if s.Code() == codes.Unavailable {
				if s.Message() == "connection closed before server preface received" {
					return status.Error(s.Code(), s.Message()+" (hint: certificate not valid?)")
				}
			}
		}
		return err
	}
	h.currentToken = response.Token
	return nil
}

func (h *Handler) retryOnInvalidTokenError(ctx context.Context, f func(token string) error) error {
	var err error
	if h.currentToken != "" {
		err = f(h.currentToken)
	} else {
		err = fmt.Errorf("%s", common.InvalidToken)
	}

	if err != nil {
		if !strings.Contains(err.Error(), common.InvalidToken) {
			return err
		}
		err2 := h.login(ctx)
		if err2 != nil {
			return err2
		}
		err = f(h.currentToken)
	}
	return err
}

func (h *Handler) getZones(cache provider.ZoneCache) (provider.DNSHostedZones, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	var remoteZones *common.Zones
	err := h.retryOnInvalidTokenError(ctx, func(token string) error {
		var err error
		h.config.RateLimiter.Accept()
		remoteZones, err = h.client.GetZones(ctx, &common.GetZonesRequest{Token: token})
		h.config.Metrics.AddGenericRequests(provider.M_LISTZONES, 1)
		return err
	})
	if err != nil {
		return nil, err
	}

	zones := provider.DNSHostedZones{}
	for _, z := range remoteZones.Zone {
		hostedZone := provider.NewDNSHostedZone(h.ProviderType(), z.Id, dns.NormalizeHostname(z.Domain), z.Key, z.ForwardedDomain, z.PrivateZone)
		zones = append(zones, hostedZone)
	}
	return zones, nil
}

func (h *Handler) GetZoneState(zone provider.DNSHostedZone) (provider.DNSZoneState, error) {
	return h.cache.GetZoneState(zone)
}

func (h *Handler) getZoneState(zone provider.DNSHostedZone, cache provider.ZoneCache) (provider.DNSZoneState, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	var remoteState *common.ZoneState
	err := h.retryOnInvalidTokenError(ctx, func(token string) error {
		var err error
		h.config.RateLimiter.Accept()
		remoteState, err = h.client.GetZoneState(ctx, &common.GetZoneStateRequest{Token: token, Zoneid: zone.Id()})
		h.config.Metrics.AddZoneRequests(zone.Id(), provider.M_LISTRECORDS, 1)
		return err
	})
	if err != nil {
		return nil, err
	}

	dnssets := conversion.UnmarshalDNSSets(remoteState.DnsSets)

	return provider.NewDNSZoneState(dnssets), nil
}

func (h *Handler) ReportZoneStateConflict(zone provider.DNSHostedZone, err error) bool {
	return h.cache.ReportZoneStateConflict(zone, err)
}

func (h *Handler) ExecuteRequests(logger logger.LogContext, zone provider.DNSHostedZone, state provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	err := h.executeRequests(logger, zone, state, reqs)
	h.cache.ApplyRequests(logger, err, zone, reqs)
	return err
}

func (h *Handler) executeRequests(logger logger.LogContext, zone provider.DNSHostedZone, state provider.DNSZoneState, reqs []*provider.ChangeRequest) error {
	if len(reqs) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Second)
	defer cancel()

	var changeRequests []*common.ChangeRequest
	for _, req := range reqs {
		change, err := conversion.MarshalChangeRequest(req)
		if err != nil {
			return err
		}
		changeRequests = append(changeRequests, change)

		switch change.Action {
		case common.ChangeRequest_CREATE | common.ChangeRequest_UPDATE:
			h.config.Metrics.AddZoneRequests(zone.Id(), provider.M_UPDATERECORDS, 1)
		case common.ChangeRequest_DELETE:
			h.config.Metrics.AddZoneRequests(zone.Id(), provider.M_DELETERECORDS, 1)
		}
	}

	var response *common.ExecuteResponse
	err := h.retryOnInvalidTokenError(ctx, func(token string) error {
		var err error
		h.config.RateLimiter.Accept()
		executeRequest := &common.ExecuteRequest{
			Token:         token,
			Zoneid:        zone.Id(),
			ChangeRequest: changeRequests,
		}
		response, err = h.client.Execute(ctx, executeRequest)
		return err
	})
	if response != nil {
		for i, changeResponse := range response.ChangeResponse {
			done := reqs[i].Done
			if done != nil {
				switch changeResponse.State {
				case common.ChangeResponse_NOT_PROCESSED:
					logger.Infof("not processed: %d", i)
				case common.ChangeResponse_SUCCEEDED:
					done.Succeeded()
				case common.ChangeResponse_INVALID:
					done.SetInvalid(fmt.Errorf("remote: %s", changeResponse.ErrorMessage))
				case common.ChangeResponse_FAILED:
					done.Failed(fmt.Errorf("remote: %s", changeResponse.ErrorMessage))
				case common.ChangeResponse_THROTTLED:
					done.Throttled()
				}
			}
		}
		for _, log := range response.LogMessage {
			ts := time.Unix(log.Timestamp/1e9, log.Timestamp%1e9)
			switch log.Level {
			case common.LogEntry_ERROR:
				logger.Errorf("%s %s", ts, log.Message)
			case common.LogEntry_WARN:
				logger.Warnf("%s %s", ts, log.Message)
			case common.LogEntry_INFO:
				logger.Infof("%s %s", ts, log.Message)
			case common.LogEntry_DEBUG:
				logger.Debugf("%s %s", ts, log.Message)
			}
		}
	}
	return err
}
