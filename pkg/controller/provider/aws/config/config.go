// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type AWSConfig struct {
	BatchSize int `json:"batchSize"`
}

// EnvAllowCredentialsChain is the operator-controlled environment variable that, when set to a
// truthy value, allows tenant secrets to request AWS_USE_CREDENTIALS_CHAIN. Without it, a tenant
// could borrow the controller's ambient AWS identity (env/web-identity token/IMDS role).
const EnvAllowCredentialsChain = "ALLOW_AWS_CREDENTIALS_CHAIN" // #nosec G101 -- false positive

// AllowCredentialsChain reports whether using AWS_USE_CREDENTIALS_CHAIN in provider secrets is enabled.
// It returns an error for a non-empty but unparseable value so the operator gets a signal instead of
// the gate silently staying closed.
func AllowCredentialsChain() (bool, error) {
	value := strings.TrimSpace(os.Getenv(EnvAllowCredentialsChain))
	if value == "" {
		return false, nil
	}
	b, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("invalid value %q for %s: %w", value, EnvAllowCredentialsChain, err)
	}
	return b, nil
}
