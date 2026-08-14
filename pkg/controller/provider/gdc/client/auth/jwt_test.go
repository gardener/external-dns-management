// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"errors"
	"testing"
)

const (
	accessToken = "test-access-token"
)

func TestJWTToken(t *testing.T) {
	saConfig := serviceAccountKey()
	jwtTS := &jwtTokenSource{
		config: saConfig,
		signer: &mockSigner{},
	}

	token, err := jwtTS.Token()
	if err != nil {
		t.Fatalf("got error: %v", err)
	}

	if token.AccessToken != accessToken {
		t.Fatalf("got access token (%s), expected (%s)", token.AccessToken, accessToken)
	}
}

func TestToken_Failed(t *testing.T) {
	saConfig := serviceAccountKey()
	jwtTS := &jwtTokenSource{
		config: saConfig,
		signer: &mockSignerError{},
	}

	_, err := jwtTS.Token()
	if err == nil {
		t.Fatal("got nil, expected error")
	}
}

func serviceAccountKey() *ServiceAccount {
	return &ServiceAccount{
		Name:         "name",
		Project:      "project",
		TokenURI:     "token_uri",
		PrivateKeyID: "1234",
		PrivateKey:   "key",
	}
}

type mockSigner struct {
}

func (m *mockSigner) signJWTWithKey(_, _, _, _, _ string) (string, error) {
	return accessToken, nil
}

type mockSignerError struct {
}

func (m *mockSignerError) signJWTWithKey(_, _, _, _, _ string) (string, error) {
	return "", errors.New("failed")
}
