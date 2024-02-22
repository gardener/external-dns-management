// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package remote

import (
	"crypto/rand"
	"encoding/base64"
)

func randonString(len int) (string, error) {
	tokenRnd := make([]byte, len)
	_, err := rand.Read(tokenRnd)
	if err != nil {
		return "", err
	}

	return base64.StdEncoding.EncodeToString(tokenRnd), nil
}

func substr(input string, start int, length int) string {
	asRunes := []rune(input)

	if start >= len(asRunes) {
		return ""
	}

	if start+length > len(asRunes) {
		length = len(asRunes) - start
	}

	return string(asRunes[start : start+length])
}
