// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package utils

import (
	"errors"

	gardencorev1beta1 "github.com/gardener/gardener/pkg/apis/core/v1beta1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Error Code Determination", func() {
	Describe("DetermineErrorCodes", func() {
		Context("when error is nil", func() {
			It("should return nil codes", func() {
				codes := DetermineErrorCodes(nil)
				Expect(codes).To(BeNil())
			})
		})

		Context("when error contains authentication keywords", func() {
			It("should return ErrorInfraUnauthenticated for invalid token", func() {
				err := errors.New("Cloudflare API error: Invalid access token (HTTP 403)")
				codes := DetermineErrorCodes(err)
				Expect(codes).To(HaveLen(1))
				Expect(codes).To(ContainElement(gardencorev1beta1.ErrorInfraUnauthenticated))
			})

			It("should return ErrorInfraUnauthenticated for invalid API key", func() {
				err := errors.New("Invalid API key provided")
				codes := DetermineErrorCodes(err)
				Expect(codes).To(ConsistOf(gardencorev1beta1.ErrorInfraUnauthenticated))
			})

			It("should return ErrorInfraUnauthenticated for bad credentials", func() {
				err := errors.New("Authentication failed: bad credentials")
				codes := DetermineErrorCodes(err)
				Expect(codes).To(ConsistOf(gardencorev1beta1.ErrorInfraUnauthenticated))
			})

			It("should return ErrorInfraUnauthenticated for Aws Route53 invalid access", func() {
				err := errors.New(`ListHostedZones failed: api error SignatureDoesNotMatch: The request
      signature we calculated does not match the signature you provided. Check your
      AWS Secret Access Key and signing method. Consult the service documentation
      for details.`)
				codes := DetermineErrorCodes(err)
				Expect(codes).To(ConsistOf(gardencorev1beta1.ErrorInfraUnauthenticated))
			})

			It("should return ErrorInfraUnauthenticated for Google Cloud DNS invalid grant", func() {
				err := errors.New(`Response: {"error":"invalid_grant","error_description":"Invalid grant: account not found"}`)
				codes := DetermineErrorCodes(err)
				Expect(codes).To(ConsistOf(gardencorev1beta1.ErrorInfraUnauthenticated))
			})
		})

		Context("when error contains authorization keywords", func() {
			It("should return ErrorInfraUnauthorized for forbidden", func() {
				err := errors.New("Access forbidden: insufficient permissions")
				codes := DetermineErrorCodes(err)
				Expect(codes).To(ConsistOf(gardencorev1beta1.ErrorInfraUnauthorized))
			})

			It("should return ErrorInfraUnauthorized for access denied", func() {
				err := errors.New("Access denied for this resource")
				codes := DetermineErrorCodes(err)
				Expect(codes).To(ConsistOf(gardencorev1beta1.ErrorInfraUnauthorized))
			})
		})

		Context("when error contains rate limiting keywords", func() {
			It("should return ErrorInfraRateLimitsExceeded for rate limit error", func() {
				err := errors.New("Rate limit exceeded, please retry later")
				codes := DetermineErrorCodes(err)
				Expect(codes).To(ConsistOf(gardencorev1beta1.ErrorInfraRateLimitsExceeded))
			})

			It("should return ErrorInfraRateLimitsExceeded for too many requests", func() {
				err := errors.New("Too many requests (HTTP 429)")
				codes := DetermineErrorCodes(err)
				Expect(codes).To(ConsistOf(gardencorev1beta1.ErrorInfraRateLimitsExceeded))
			})

			It("should return ErrorInfraRateLimitsExceeded for throttling", func() {
				err := errors.New("Request throttled by provider")
				codes := DetermineErrorCodes(err)
				Expect(codes).To(ConsistOf(gardencorev1beta1.ErrorInfraRateLimitsExceeded))
			})
		})

		Context("when error contains quota keywords", func() {
			It("should return ErrorInfraQuotaExceeded for quota error", func() {
				err := errors.New("Quota exceeded for DNS zones")
				codes := DetermineErrorCodes(err)
				Expect(codes).To(ConsistOf(gardencorev1beta1.ErrorInfraQuotaExceeded))
			})
		})

		Context("when error contains configuration keywords", func() {
			It("should return ErrorConfigurationProblem for malformed request", func() {
				err := errors.New("Malformed API request")
				codes := DetermineErrorCodes(err)
				Expect(codes).To(ConsistOf(gardencorev1beta1.ErrorConfigurationProblem))
			})

			It("should return ErrorConfigurationProblem for unsupported provider", func() {
				err := errors.New("provider type \"foo\" is not supported")
				codes := DetermineErrorCodes(err)
				Expect(codes).To(ConsistOf(gardencorev1beta1.ErrorConfigurationProblem))
			})
		})

		Context("when error contains multiple error patterns", func() {
			It("should return multiple error codes", func() {
				err := errors.New("Invalid token and rate limit exceeded")
				codes := DetermineErrorCodes(err)
				Expect(codes).To(HaveLen(2))
				Expect(codes).To(ConsistOf(
					gardencorev1beta1.ErrorInfraUnauthenticated,
					gardencorev1beta1.ErrorInfraRateLimitsExceeded,
				))
			})
		})

		Context("when error does not match any pattern", func() {
			It("should return no error codes", func() {
				err := errors.New("Generic error message without keywords")
				codes := DetermineErrorCodes(err)
				Expect(codes).To(BeEmpty())
			})
		})
	})

	Describe("hasNonRetryableErrorCode", func() {
		Context("when codes list is empty", func() {
			It("should return false for nil codes", func() {
				Expect(HasNonRetryableErrorCode(nil)).To(BeFalse())
			})

			It("should return false for empty codes slice", func() {
				Expect(HasNonRetryableErrorCode([]gardencorev1beta1.ErrorCode{})).To(BeFalse())
			})
		})

		Context("when codes contain only retryable errors", func() {
			It("should return false for rate limit error", func() {
				codes := []gardencorev1beta1.ErrorCode{gardencorev1beta1.ErrorInfraRateLimitsExceeded}
				Expect(HasNonRetryableErrorCode(codes)).To(BeFalse())
			})

			It("should return false for retryable configuration problem", func() {
				codes := []gardencorev1beta1.ErrorCode{gardencorev1beta1.ErrorRetryableConfigurationProblem}
				Expect(HasNonRetryableErrorCode(codes)).To(BeFalse())
			})

			It("should return false for retryable infra dependencies", func() {
				codes := []gardencorev1beta1.ErrorCode{gardencorev1beta1.ErrorRetryableInfraDependencies}
				Expect(HasNonRetryableErrorCode(codes)).To(BeFalse())
			})
		})

		Context("when codes contain non-retryable errors", func() {
			It("should return true for unauthenticated error", func() {
				codes := []gardencorev1beta1.ErrorCode{gardencorev1beta1.ErrorInfraUnauthenticated}
				Expect(HasNonRetryableErrorCode(codes)).To(BeTrue())
			})

			It("should return true for unauthorized error", func() {
				codes := []gardencorev1beta1.ErrorCode{gardencorev1beta1.ErrorInfraUnauthorized}
				Expect(HasNonRetryableErrorCode(codes)).To(BeTrue())
			})

			It("should return true for configuration problem", func() {
				codes := []gardencorev1beta1.ErrorCode{gardencorev1beta1.ErrorConfigurationProblem}
				Expect(HasNonRetryableErrorCode(codes)).To(BeTrue())
			})

			It("should return true for quota exceeded error", func() {
				codes := []gardencorev1beta1.ErrorCode{gardencorev1beta1.ErrorInfraQuotaExceeded}
				Expect(HasNonRetryableErrorCode(codes)).To(BeTrue())
			})
		})

		Context("when codes contain mixed retryable and non-retryable errors", func() {
			It("should return true if any code is non-retryable", func() {
				codes := []gardencorev1beta1.ErrorCode{
					gardencorev1beta1.ErrorInfraRateLimitsExceeded,
					gardencorev1beta1.ErrorInfraUnauthenticated,
				}
				Expect(HasNonRetryableErrorCode(codes)).To(BeTrue())
			})
		})
	})

	Describe("containsAny", func() {
		Context("when substring list is empty", func() {
			It("should return false", func() {
				Expect(containsAny("hello world")).To(BeFalse())
			})
		})

		Context("when string contains one of the substrings", func() {
			It("should return true for single match", func() {
				Expect(containsAny("invalid token provided", "invalid", "error")).To(BeTrue())
			})

			It("should return true for multiple matches", func() {
				Expect(containsAny("invalid token provided", "invalid", "token")).To(BeTrue())
			})
		})

		Context("when string contains none of the substrings", func() {
			It("should return false", func() {
				Expect(containsAny("hello world", "foo", "bar")).To(BeFalse())
			})

			It("should be case sensitive", func() {
				Expect(containsAny("hello world", "Hello")).To(BeFalse())
			})
		})
	})
})
