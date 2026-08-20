// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"golang.org/x/oauth2"
	"k8s.io/client-go/transport"
	"k8s.io/utils/clock"
)

const (
	tokenExchangeType      = "urn:ietf:params:oauth:token-type:token-exchange" // #nosec G101 -- false positive
	accessTokenType        = "urn:ietf:params:oauth:token-type:access_token"   // #nosec G101 -- false positive
	serviceAccoutTokenType = "urn:k8s:params:oauth:token-type:serviceaccount"  // #nosec G101 -- false positive
)

// Option configures optional parameters for STS token source creation.
type Option func(*optionalConfig)

// WithCACert sets the CA certificate to be used for server certificate
// validation.
func WithCACert(caCert []byte) Option {
	return func(cfg *optionalConfig) {
		cfg.caCert = caCert
	}
}

// NewCachedSTSTokenSource creates an oauth2.TokenSource that caches STS tokens for the specified audience.
// The CachedSTSTokenSource will request a new STS token when the STS token is
// expired or nearing the expiry time.
func NewCachedSTSTokenSource(
	audience string,
	saConfig *ServiceAccount,
	opts ...Option,
) oauth2.TokenSource {
	stsTS := NewSTSTokenSource(audience, saConfig, opts...)
	return transport.NewCachedTokenSource(stsTS)
}

type optionalConfig struct {
	caCert []byte
}

// NewSTSTokenSource creates an oauth2.TokenSource for STS token exchange.
func NewSTSTokenSource(
	audience string,
	saConfig *ServiceAccount,
	opts ...Option,
) oauth2.TokenSource {
	cfg := &optionalConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	jwtTS := newJWTTokenSource(saConfig)
	return &stsTokenSource{
		caCert:         cfg.caCert,
		tokenURI:       saConfig.TokenURI,
		audience:       audience,
		jwtTokenSource: jwtTS,
		clock:          clock.RealClock{},
	}
}

type stsTokenSource struct {
	caCert         []byte
	tokenURI       string
	audience       string
	jwtTokenSource oauth2.TokenSource
	clock          clock.PassiveClock
}

type generateAccessTokenResp struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int64  `json:"expires_in"` // relative seconds from now
}

// Token exchanges signed JWT token for STS token.
func (ts *stsTokenSource) Token() (*oauth2.Token, error) {
	jwtToken, err := ts.jwtTokenSource.Token()
	if err != nil {
		return nil, fmt.Errorf("jwt token: %w", err)
	}

	data := map[string]string{
		"grant_type":           tokenExchangeType,
		"audience":             ts.audience,
		"requested_token_type": accessTokenType,
		"subject_token":        jwtToken.AccessToken,
		"subject_token_type":   serviceAccoutTokenType,
	}
	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, fmt.Errorf("marshal request body: %w", err)
	}

	body := bytes.NewReader(jsonData)
	req, err := http.NewRequest("POST", ts.tokenURI, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	hc := http.DefaultClient

	if len(ts.caCert) != 0 {
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM(ts.caCert)
		hc.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs: caCertPool,
			},
		}
	}

	resp, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch token: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read fetched token: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, &oauth2.RetrieveError{
			Response: resp,
			Body:     respBody,
		}
	}

	var tokenResp generateAccessTokenResp
	if err := json.Unmarshal(respBody, &tokenResp); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	expiry := ts.clock.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	return &oauth2.Token{
		AccessToken: tokenResp.AccessToken,
		TokenType:   "Bearer",
		Expiry:      expiry,
	}, nil
}
