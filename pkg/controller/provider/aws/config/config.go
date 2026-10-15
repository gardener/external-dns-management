// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package config

import (
	"os"
	"strconv"
)

type AWSConfig struct {
	BatchSize int `json:"batchSize"`
}

// EnvAllowCredentialsChain is the operator-controlled environment variable that, when set to a
// truthy value, allows tenant secrets to request AWS_USE_CREDENTIALS_CHAIN. Without it, a tenant
// could borrow the controller's ambient AWS identity (env/web-identity token/IMDS role).
const EnvAllowCredentialsChain = "ALLOW_AWS_CREDENTIALS_CHAIN"

// AllowCredentialsChain reports whether using AWS_USE_CREDENTIALS_CHAIN in provider secrets is enabled.
func AllowCredentialsChain() bool {
	b, _ := strconv.ParseBool(os.Getenv(EnvAllowCredentialsChain))
	return b
}
