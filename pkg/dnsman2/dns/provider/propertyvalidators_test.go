// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"regexp"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Property Validators", func() {
	Describe("AlphaNumericValidator", func() {
		It("accepts alphanumeric", func() {
			Expect(AlphaNumericValidator("abc123")).To(Succeed())
		})
		It("rejects non-alphanumeric", func() {
			Expect(AlphaNumericValidator("abc-123")).ToNot(Succeed())
		})
	})

	Describe("AlphaNumericPunctuationValidator", func() {
		It("accepts allowed punctuation", func() {
			Expect(AlphaNumericPunctuationValidator("abc-123_:.")).To(Succeed())
		})
		It("rejects disallowed chars", func() {
			Expect(AlphaNumericPunctuationValidator("abc/123")).ToNot(Succeed())
		})
	})

	Describe("PrintableValidator", func() {
		It("accepts printable unicode", func() {
			Expect(PrintableValidator("abc-123_:.äöüß€")).To(Succeed())
		})
		It("rejects non-printable chars", func() {
			Expect(PrintableValidator("abc\x00def")).ToNot(Succeed())
		})
	})

	Describe("Base64CharactersValidator", func() {
		It("accepts base64", func() {
			Expect(Base64CharactersValidator("YWJjMTIz+/=")).To(Succeed())
		})
		It("rejects non-base64", func() {
			Expect(Base64CharactersValidator("abc*123")).ToNot(Succeed())
		})
	})

	Describe("BoolValidator", func() {
		It("accepts true/false", func() {
			Expect(BoolValidator("true")).To(Succeed())
			Expect(BoolValidator("false")).To(Succeed())
		})
		It("rejects non-bool", func() {
			Expect(BoolValidator("yes")).ToNot(Succeed())
		})
	})

	Describe("RegExValidator", func() {
		It("accepts matching regex", func() {
			v := RegExValidator(regexp.MustCompile(`^foo\d+$`))
			Expect(v("foo123")).To(Succeed())
		})
		It("rejects non-matching", func() {
			v := RegExValidator(regexp.MustCompile(`^foo\d+$`))
			Expect(v("bar123")).ToNot(Succeed())
		})
	})

	Describe("MaxLengthValidator", func() {
		It("accepts within max", func() {
			v := MaxLengthValidator(5)
			Expect(v("12345")).To(Succeed())
		})
		It("rejects over max", func() {
			v := MaxLengthValidator(3)
			Expect(v("abcd")).ToNot(Succeed())
		})
	})

	Describe("PredefinedValuesValidator", func() {
		It("accepts predefined", func() {
			v := PredefinedValuesValidator("foo", "bar")
			Expect(v("foo")).To(Succeed())
		})
		It("rejects not predefined", func() {
			v := PredefinedValuesValidator("foo", "bar")
			Expect(v("baz")).ToNot(Succeed())
		})
	})

	Describe("IntValidator", func() {
		It("accepts in range", func() {
			v := IntValidator(1, 10)
			Expect(v("5")).To(Succeed())
		})
		It("rejects out of range", func() {
			v := IntValidator(1, 10)
			Expect(v("11")).ToNot(Succeed())
		})
		It("rejects non-int", func() {
			v := IntValidator(1, 10)
			Expect(v("abc")).ToNot(Succeed())
		})
	})

	Describe("URLValidator", func() {
		It("accepts allowed scheme", func() {
			v := URLValidator("https", "http")
			Expect(v("https://example.com")).To(Succeed())
		})
		It("rejects not allowed scheme", func() {
			v := URLValidator("https")
			Expect(v("ftp://example.com")).ToNot(Succeed())
		})
		It("rejects invalid url", func() {
			v := URLValidator("https")
			Expect(v("https://%zz")).ToNot(Succeed())
		})
	})

	Describe("NoTrailingWhitespaceValidator", func() {
		It("accepts no trailing whitespace", func() {
			Expect(NoTrailingWhitespaceValidator("foo")).To(Succeed())
		})
		It("rejects trailing whitespace", func() {
			Expect(NoTrailingWhitespaceValidator("foo ")).ToNot(Succeed())
		})
	})

	Describe("NoTrailingNewlineValidator", func() {
		It("accepts no trailing newline", func() {
			Expect(NoTrailingNewlineValidator("foo")).To(Succeed())
		})
		It("rejects trailing newline", func() {
			Expect(NoTrailingNewlineValidator("foo\n")).ToNot(Succeed())
		})
	})

	Describe("PEMValidator", func() {
		It("accepts valid PEM", func() {
			block := &pem.Block{Type: "CERTIFICATE", Bytes: []byte("test")}
			pemStr := string(pem.EncodeToMemory(block))
			Expect(PEMValidator(pemStr)).To(Succeed())
		})

		It("rejects invalid PEM", func() {
			Expect(PEMValidator("not a pem")).ToNot(Succeed())
		})
	})

	Describe("CACertValidator", func() {
		It("accepts valid CA PEM", func() {
			cacert, err := generateCACert()
			Expect(err).ToNot(HaveOccurred())
			Expect(CACertValidator(cacert)).To(Succeed())
		})

		It("rejects invalid PEM", func() {
			Expect(CACertValidator("not a pem")).ToNot(Succeed())
		})
	})

	Describe("ExpectedValueValidator", func() {
		It("accepts expected value", func() {
			Expect(ExpectedValueValidator("foo")("foo")).To(Succeed())
		})

		It("rejects unexpected value", func() {
			Expect(ExpectedValueValidator("foo")("bar")).ToNot(Succeed())
		})
	})
})

func generateCACert() (string, error) {
	ca := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Country:            []string{"DE"},
			Organization:       []string{"SAP SE"},
			OrganizationalUnit: []string{"Gardener External DNS Management"},
			CommonName:         "",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(2 * time.Hour),
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, 4096)
	if err != nil {
		return "", err
	}

	crtBytes, err := x509.CreateCertificate(rand.Reader, ca, ca, &privateKey.PublicKey, privateKey)
	if err != nil {
		return "", err
	}
	cacert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: crtBytes})

	return string(cacert), nil
}
