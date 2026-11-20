// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package client

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"k8s.io/client-go/rest"

	"github.com/gardener/external-dns-management/pkg/controller/provider/gdc/client/auth"
)

func generateTestECPrivateKeyPEM(t *testing.T) string {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate ECDSA key: %v", err)
	}
	der, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		t.Fatalf("marshal ECDSA key: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der}))
}

func TestGetRESTConfig_ResetsCachedTokenOn401(t *testing.T) {
	var stsCalls atomic.Int32
	stsServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		n := stsCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": fmt.Sprintf("sts-token-%d", n),
			"expires_in":   43200,
		})
	}))
	defer stsServer.Close()

	var invalidateFirstToken atomic.Bool
	var lastAuthHeader atomic.Value
	apiServer := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHdr := r.Header.Get("Authorization")
		lastAuthHeader.Store(authHdr)
		if invalidateFirstToken.Load() && authHdr == "Bearer sts-token-1" {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(`{"kind":"Status","status":"Failure","reason":"Unauthorized","code":401}`))
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer apiServer.Close()

	var caBundle []byte
	for _, cert := range []*x509.Certificate{stsServer.Certificate(), apiServer.Certificate()} {
		caBundle = append(caBundle, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw})...)
	}

	sa := &auth.ServiceAccount{
		Name:         "test-sa",
		Project:      "test-project",
		PrivateKeyID: "test-key-id",
		PrivateKey:   generateTestECPrivateKeyPEM(t),
		TokenURI:     stsServer.URL,
	}

	restCfg, err := getRESTConfig(&OrgClusterConfig{
		OrgClusterURL: apiServer.URL,
		CAData:        base64.StdEncoding.EncodeToString(caBundle),
	}, sa)
	if err != nil {
		t.Fatalf("getRESTConfig: %v", err)
	}

	httpClient, err := rest.HTTPClientFor(restCfg)
	if err != nil {
		t.Fatalf("rest.HTTPClientFor: %v", err)
	}

	doRequest := func() int {
		t.Helper()
		resp, err := httpClient.Get(apiServer.URL + "/apis/dns.global.gdc.goog/v1/namespaces/test/manageddnszones")
		if err != nil {
			t.Fatalf("GET failed: %v", err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		return resp.StatusCode
	}

	if got := doRequest(); got != http.StatusOK {
		t.Fatalf("request 1 status = %d, want 200", got)
	}
	if got := stsCalls.Load(); got != 1 {
		t.Fatalf("after request 1 stsCalls = %d, want 1", got)
	}
	if got := lastAuthHeader.Load().(string); got != "Bearer sts-token-1" {
		t.Fatalf("request 1 auth = %q, want Bearer sts-token-1", got)
	}

	if got := doRequest(); got != http.StatusOK {
		t.Fatalf("request 2 status = %d, want 200", got)
	}
	if got := stsCalls.Load(); got != 1 {
		t.Fatalf("after request 2 stsCalls = %d, want 1 (cached)", got)
	}

	time.Sleep(2 * time.Millisecond)
	invalidateFirstToken.Store(true)

	if got := doRequest(); got != http.StatusUnauthorized {
		t.Fatalf("request 3 status = %d, want 401", got)
	}
	if got := stsCalls.Load(); got != 1 {
		t.Fatalf("after request 3 stsCalls = %d, want 1", got)
	}

	if got := doRequest(); got != http.StatusOK {
		t.Fatalf("request 4 status = %d, want 200 (after 401 token reset)", got)
	}
	if got := stsCalls.Load(); got != 2 {
		t.Fatalf("after request 4 stsCalls = %d, want 2", got)
	}
	if got := lastAuthHeader.Load().(string); got != "Bearer sts-token-2" {
		t.Fatalf("request 4 auth = %q, want Bearer sts-token-2", got)
	}

	if got := doRequest(); got != http.StatusOK {
		t.Fatalf("request 5 status = %d, want 200", got)
	}
	if got := stsCalls.Load(); got != 2 {
		t.Fatalf("after request 5 stsCalls = %d, want 2 (cached)", got)
	}
}
