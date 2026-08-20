// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"golang.org/x/oauth2"
	clock "k8s.io/utils/clock/testing"
)

const (
	jwtAccessToken   = "test-jwt-access-token"
	stsAccessToken   = "test-sts-access-token"
	expiresInSeconds = 1700434861
)

var testTimeNow = time.Date(2023, 1, 1, 1, 1, 1, 1, time.UTC)

func TestSTSToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, mockResponse())
	}))
	defer server.Close()

	stsTS := &stsTokenSource{
		caCert:         []byte("caData"),
		tokenURI:       server.URL,
		audience:       "audience",
		jwtTokenSource: &mockJWTTokenSource{},
		clock:          clock.NewFakePassiveClock(testTimeNow),
	}

	got, err := stsTS.Token()
	if err != nil {
		t.Fatalf("got error: %v", err)
	}

	expected := &oauth2.Token{
		AccessToken: stsAccessToken,
		TokenType:   "Bearer",
		Expiry:      testTimeNow.Add(time.Duration(expiresInSeconds) * time.Second),
	}

	if diff := cmp.Diff(expected, got, cmpopts.IgnoreUnexported(oauth2.Token{})); diff != "" {
		t.Errorf("test requests diff (-want +got):\n%s", diff)
	}
}

func TestSTSToken_Failed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, mockResponse())
	}))
	defer server.Close()

	stsTS := &stsTokenSource{
		caCert:         []byte("caData"),
		tokenURI:       server.URL,
		audience:       "audience",
		jwtTokenSource: &mockJWTTokenSource{},
	}

	_, err := stsTS.Token()
	if err == nil {
		t.Fatal("got nil, expected error")
	}
}

type mockJWTTokenSource struct {
}

func (mts *mockJWTTokenSource) Token() (*oauth2.Token, error) {
	return &oauth2.Token{AccessToken: jwtAccessToken}, nil
}

func mockResponse() string {
	tokenResp := &generateAccessTokenResp{
		AccessToken: stsAccessToken,
		ExpiresIn:   expiresInSeconds,
	}

	jsonData, _ := json.Marshal(tokenResp)
	return string(jsonData)
}
