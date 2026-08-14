// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"

	"github.com/google/tink/go/insecurecleartextkeyset"
	"github.com/google/tink/go/jwt"
	"github.com/google/tink/go/keyset"
	"github.com/google/tink/go/proto/jwt_ecdsa_go_proto"
	"github.com/google/tink/go/proto/tink_go_proto"
	"golang.org/x/oauth2"
	"google.golang.org/protobuf/proto"
	"k8s.io/utils/ptr"
)

type jwtSigner interface {
	signJWTWithKey(kid, key, sub, issuer, audience string) (string, error)
}

type defaultSigner struct{}

func (s *defaultSigner) signJWTWithKey(kid, key, sub, issuer, audience string) (string, error) {
	return signJWTWithKey(kid, key, sub, issuer, audience)
}

func newJWTTokenSource(config *ServiceAccount) oauth2.TokenSource {
	return &jwtTokenSource{
		config: config,
		signer: &defaultSigner{},
	}
}

type jwtTokenSource struct {
	config *ServiceAccount
	signer jwtSigner
}

func (ts *jwtTokenSource) Token() (*oauth2.Token, error) {
	// The service account name is both the issuer and the subject of the signed JWT.
	issSubValue := fmt.Sprintf("system:serviceaccount:%s:%s", ts.config.Project, ts.config.Name)
	jwtToken, err := ts.signer.signJWTWithKey(ts.config.PrivateKeyID, ts.config.PrivateKey, issSubValue, issSubValue, ts.config.TokenURI)
	if err != nil {
		return nil, fmt.Errorf("jwt token: %w", err)
	}

	// No need to populate expiry as JWT is for token exchange
	return &oauth2.Token{AccessToken: jwtToken}, nil
}

// signJWTWithKey signs a JWT using a known private key to be used with
// the service identity server in order to exchange for an audienced
// STS token. This only supports signing the JWT with ECDSA 256 keys.
func signJWTWithKey(kid, key, sub, issuer, audience string) (string, error) {
	priv, err := parsePrivateKey(key)
	if err != nil {
		return "", err
	}

	privKey := &jwt_ecdsa_go_proto.JwtEcdsaPrivateKey{
		PublicKey: &jwt_ecdsa_go_proto.JwtEcdsaPublicKey{
			Algorithm: jwt_ecdsa_go_proto.JwtEcdsaAlgorithm_ES256,
			X:         priv.X.Bytes(),
			Y:         priv.Y.Bytes(),
			CustomKid: &jwt_ecdsa_go_proto.JwtEcdsaPublicKey_CustomKid{Value: kid},
		},
		//nolint:staticcheck // priv.D is required for tink ES256 protobuf key serialization
		KeyValue: priv.D.Bytes(),
	}
	serializedKey, err := proto.Marshal(privKey)
	if err != nil {
		return "", err
	}

	ks := &tink_go_proto.Keyset{
		PrimaryKeyId: 1,
		Key: []*tink_go_proto.Keyset_Key{
			{
				KeyData: &tink_go_proto.KeyData{
					TypeUrl:         jwt.ES256Template().TypeUrl,
					Value:           serializedKey,
					KeyMaterialType: tink_go_proto.KeyData_ASYMMETRIC_PRIVATE,
				},
				Status:           tink_go_proto.KeyStatusType_ENABLED,
				KeyId:            1,
				OutputPrefixType: tink_go_proto.OutputPrefixType_RAW,
			},
		},
	}
	serializedKs, err := proto.Marshal(ks)
	if err != nil {
		return "", err
	}

	// The private key is already stored on the disk. It should be safe to use
	// insecurecleartext because the only endpoint the JWT is used with is the
	// service identity server which verifies the signature of the JWT using the
	// public key.
	handle, err := insecurecleartextkeyset.Read(keyset.NewBinaryReader(bytes.NewBuffer(serializedKs)))
	if err != nil {
		return "", err
	}

	token, err := generateJWT(handle, issuer, audience, sub)
	if err != nil {
		return "", err
	}
	return token, nil
}

// generateJWT generates a signed JWT for the requested issuer, audience, and subject.
func generateJWT(handle *keyset.Handle, issuer, audience, subject string) (string, error) {
	jwtOpts := &jwt.RawJWTOptions{
		Issuer:     ptr.To(issuer),
		Subject:    ptr.To(subject),
		Audience:   ptr.To(audience),
		ExpiresAt:  refTime(time.Now().Add(time.Hour * 24)),
		IssuedAt:   refTime(time.Now()),
		TypeHeader: ptr.To("JWT"),
	}

	rawJWT, err := jwt.NewRawJWT(jwtOpts)
	if err != nil {
		return "", fmt.Errorf("could not create raw JWT: %v", err)
	}

	signer, err := jwt.NewSigner(handle)
	if err != nil {
		return "", fmt.Errorf("failed to create signer: %v", err)
	}
	jwtString, err := signer.SignAndEncode(rawJWT)
	if err != nil {
		return "", fmt.Errorf("failed to sign and encode JWT: %v", err)
	}
	return jwtString, nil
}

// parsePrivateKey parses a private key string into an *ecdsa.PrivateKey.
func parsePrivateKey(key string) (*ecdsa.PrivateKey, error) {
	block, _ := pem.Decode([]byte(key))
	if block == nil {
		return nil, fmt.Errorf("private key decoded into nil PEM block")
	}
	return x509.ParseECPrivateKey(block.Bytes)
}

func refTime(t time.Time) *time.Time {
	return &t
}
