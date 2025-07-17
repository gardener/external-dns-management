// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// PropertyValidator defines a function type for validating property values.
type PropertyValidator func(value string) error

var alphaNumRegex = regexp.MustCompile(`^[a-zA-Z0-9]+$`)

// AlphaNumericValidator checks if the value contains only alphanumeric characters.
func AlphaNumericValidator(value string) error {
	if !alphaNumRegex.MatchString(value) {
		return fmt.Errorf("value must contain only alphanumeric characters")
	}
	return nil
}

var alphaNumPunctationRegex = regexp.MustCompile(`^[a-zA-Z0-9._:-]+$`)

// AlphaNumericPunctuationValidator checks if the value contains only alphanumeric characters, dots, hyphens, underscores, and colons.
func AlphaNumericPunctuationValidator(value string) error {
	if !alphaNumPunctationRegex.MatchString(value) {
		return fmt.Errorf("value must contain only alphanumeric characters, dots, hyphens, underscores, and colons")
	}
	return nil
}

// PrintableValidator checks if the value contains only printable Unicode characters.
func PrintableValidator(value string) error {
	for _, r := range value {
		if !strconv.IsPrint(r) {
			return fmt.Errorf("value must contain only printable characters, found: rune %d", r)
		}
	}
	return nil
}

var base64Regex = regexp.MustCompile(`^[a-zA-Z0-9=+/]+$`)

// Base64CharactersValidator checks if the value contains only characters used for base64 encoding.
func Base64CharactersValidator(value string) error {
	if !base64Regex.MatchString(value) {
		return fmt.Errorf("value must contain only characters used for base64 encoding (A-Z, a-z, 0-9, +, /, and =)")
	}
	return nil
}

// BoolValidator checks if the value is a valid boolean (true/false).
func BoolValidator(value string) error {
	if _, err := strconv.ParseBool(value); err != nil {
		return fmt.Errorf("value must be a boolean (true/false)")
	}
	return nil
}

// RegExValidator checks if the value matches the provided regular expression.
func RegExValidator(r *regexp.Regexp) PropertyValidator {
	return func(value string) error {
		if !r.MatchString(value) {
			return fmt.Errorf("value must follow regular expresion: %s", r.String())
		}
		return nil
	}
}

// MaxLengthValidator checks if the length of the value does not exceed the specified maximum.
func MaxLengthValidator(max int) PropertyValidator {
	return func(value string) error {
		if len(value) > max {
			return fmt.Errorf("value exceeds maximum length of %d characters", max)
		}
		return nil
	}
}

// PredefinedValuesValidator checks if the value is one of the predefined values.
func PredefinedValuesValidator(values ...string) PropertyValidator {
	return func(value string) error {
		for _, v := range values {
			if value == v {
				return nil
			}
		}
		return fmt.Errorf("value must be one of: %s", strings.Join(values, ", "))
	}
}

// IntValidator checks if the value is an integer within the specified range.
func IntValidator(min, max int) PropertyValidator {
	return func(value string) error {
		v, err := strconv.Atoi(value)
		if err != nil {
			return fmt.Errorf("value must be an integer")
		}
		if v < min || v > max {
			return fmt.Errorf("value must be between %d and %d", min, max)
		}
		return nil
	}
}

// URLValidator checks if the value is a valid URL and starts with one of the specified schemes.
func URLValidator(schemes ...string) PropertyValidator {
	return func(value string) error {
		allowedScheme := false
		for _, scheme := range schemes {
			if strings.HasPrefix(value, scheme+"://") {
				allowedScheme = true
				break
			}
		}
		if !allowedScheme {
			return fmt.Errorf("value must start with one of the following schemes: %s", strings.Join(schemes, ", "))
		}
		_, err := url.Parse(value)
		if err != nil {
			return fmt.Errorf("value must be a valid URL, error: %s", err)
		}
		return nil
	}
}

// CACertValidator checks if the value is a valid list of CA certificates in PEM format.
func CACertValidator(value string) error {
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM([]byte(value)) {
		return fmt.Errorf("cannot parse certificates from PEM format")
	}
	return nil
}

// PEMValidator checks if the value is a valid PEM encoded string.
func PEMValidator(value string) error {
	p, _ := pem.Decode([]byte(value))
	if p == nil {
		return fmt.Errorf("value must be a valid PEM encoded string")
	}
	return nil
}

// NoTrailingWhitespaceValidator checks if the value does not contain trailing whitespace.
func NoTrailingWhitespaceValidator(value string) error {
	if strings.TrimSpace(value) != value {
		return fmt.Errorf("value must not contain trailing whitespace")
	}
	return nil
}

// NoNewlineValidator checks if the value does not contain any trailing newline characters.
func NoTrailingNewlineValidator(value string) error {
	if strings.Trim(value, "\n\r") != value {
		return fmt.Errorf("value must not contain newlines")
	}
	return nil
}
