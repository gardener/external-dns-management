// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

// Package main provides a bootstrap CLI helper that exchanges a GDC
// ServiceAccount JSON key for an STS Bearer token scoped to a target audience URL.
// This is used in CI before the gdcloud CLI is installed to authenticate against
// the GDC Management API server and query CLIBundleMetadata.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/gardener/external-dns-management/pkg/controller/provider/gdc/client/auth"
)

func main() {
	var (
		saPath   string
		caPath   string
		audience string
	)
	flag.StringVar(&saPath, "service-account-file", "", "Path to the GDC ServiceAccount JSON key file.")
	flag.StringVar(&caPath, "ca-cert-file", "", "Path to the GDC Root CA certificate PEM file.")
	flag.StringVar(&audience, "audience", "", "Target audience URL for the STS token exchange (e.g. GDC Management API URL).")
	flag.Parse()

	token, err := mintSTSToken(saPath, caPath, audience)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		flag.Usage()
		os.Exit(1)
	}

	// Print the raw access token to stdout so the caller script can capture it
	// for Authorization: Bearer headers.
	fmt.Print(token)
}

// mintSTSToken reads the GDC ServiceAccount key and Root CA certificate from disk
// and exchanges a locally signed ES256 JWT for an STS access token.
func mintSTSToken(saPath, caPath, audience string) (string, error) {
	if saPath == "" || caPath == "" || audience == "" {
		return "", fmt.Errorf("--service-account-file, --ca-cert-file, and --audience are all required")
	}

	// Read and parse the GDC ProjectServiceAccount JSON credential file, which
	// contains the ES256 private key, key ID, project, service account name, and STS token URI.
	saBytes, err := os.ReadFile(saPath) // #nosec G304 -- CLI helper reading SA file
	if err != nil {
		return "", fmt.Errorf("read service account file %q: %w", saPath, err)
	}

	var sa auth.ServiceAccount
	if err := json.Unmarshal(saBytes, &sa); err != nil {
		return "", fmt.Errorf("unmarshal service account JSON: %w", err)
	}

	// Read the GDC Root CA certificate so the STS HTTPS client can verify the
	// GDC ServiceIdentityServer TLS certificate.
	caBytes, err := os.ReadFile(caPath) // #nosec G304 -- CLI helper reading CA file
	if err != nil {
		return "", fmt.Errorf("read CA certificate file %q: %w", caPath, err)
	}

	// Create an STS TokenSource configured with the GDC Root CA and exchange a
	// signed JWT for a short-lived OAuth 2.0 Bearer access token scoped to the audience.
	ts := auth.NewSTSTokenSource(audience, &sa, auth.WithCACert(caBytes))
	tok, err := ts.Token()
	if err != nil {
		return "", fmt.Errorf("mint STS token for audience %q: %w", audience, err)
	}

	return tok.AccessToken, nil
}
