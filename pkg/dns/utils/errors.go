// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"strings"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
)

// DetermineErrorCodes analyzes an error message and returns appropriate Gardener error codes.
// This enables Gardener to distinguish between user configuration errors and system-level failures,
// allowing for proper retry logic and error propagation to Shoot status.
func DetermineErrorCodes(err error) []gardencorev1beta1.ErrorCode {
	if err == nil {
		return nil
	}

	errStr := strings.ToLower(err.Error())
	var codes []gardencorev1beta1.ErrorCode

	// Check for authentication/authorization issues
	// These are typically user configuration errors (invalid credentials/tokens)
	if containsAny(errStr,
		"unauthenticated",
		"authentication failed",
		"invalid token",
		"invalid credentials",
		"invalid api token",
		"invalid access token",
		"invalid api key",
		"authentication error",
		"auth failed",
		"bad credentials",
		"signaturedoesnotmatch",
		"invalid_grant") {
		codes = append(codes, gardencorev1beta1.ErrorInfraUnauthenticated)
	}

	if containsAny(errStr,
		"unauthorized",
		"forbidden",
		"access denied",
		"permission denied",
		"insufficient permissions") {
		codes = append(codes, gardencorev1beta1.ErrorInfraUnauthorized)
	}

	// Rate limiting - these are retryable errors
	if containsAny(errStr,
		"rate limit",
		"ratelimit",
		"throttl",
		"too many requests",
		"request limit exceeded") {
		codes = append(codes, gardencorev1beta1.ErrorInfraRateLimitsExceeded)
	}

	// Quota issues - non-retryable, user needs to increase quota
	if containsAny(errStr,
		"quota exceeded",
		"quota limit",
		"insufficient quota") {
		codes = append(codes, gardencorev1beta1.ErrorInfraQuotaExceeded)
	}

	// Generic configuration problem if no specific code matched but looks like config issue
	if len(codes) == 0 && containsAny(errStr,
		"invalid",
		"malformed",
		"bad request",
		"configuration error",
		"config error",
		"validation error",
		"validation failed",
		"not supported") {
		codes = append(codes, gardencorev1beta1.ErrorConfigurationProblem)
	}

	return codes
}

var nonRetryableCodes = map[gardencorev1beta1.ErrorCode]bool{
	gardencorev1beta1.ErrorInfraUnauthenticated:   true,
	gardencorev1beta1.ErrorInfraUnauthorized:      true,
	gardencorev1beta1.ErrorInfraQuotaExceeded:     true,
	gardencorev1beta1.ErrorInfraDependencies:      true,
	gardencorev1beta1.ErrorInfraResourcesDepleted: true,
	gardencorev1beta1.ErrorConfigurationProblem:   true,
	gardencorev1beta1.ErrorProblematicWebhook:     true,
}

// HasNonRetryableErrorCode checks if any of the provided error codes indicate
// a non-retryable error (e.g., authentication failure, configuration problem).
func HasNonRetryableErrorCode(codes []gardencorev1beta1.ErrorCode) bool {
	for _, code := range codes {
		if nonRetryableCodes[code] {
			return true
		}
	}
	return false
}

// containsAny checks if the string contains any of the provided substrings.
func containsAny(s string, substrs ...string) bool {
	for _, substr := range substrs {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}
