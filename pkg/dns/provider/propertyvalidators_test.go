// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"math/big"
	"regexp"
	"time"

	"github.com/gardener/controller-manager-library/pkg/utils/pkiutil"
	g "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gardener/external-dns-management/pkg/controller/remoteaccesscertificates"
)

var _ = g.Describe("Property Validators", func() {
	g.Describe("AlphaNumericValidator", func() {
		g.It("accepts alphanumeric", func() {
			Expect(AlphaNumericValidator("abc123")).To(Succeed())
		})
		g.It("rejects non-alphanumeric", func() {
			Expect(AlphaNumericValidator("abc-123")).ToNot(Succeed())
		})
	})

	g.Describe("AlphaNumericPunctuationValidator", func() {
		g.It("accepts allowed punctuation", func() {
			Expect(AlphaNumericPunctuationValidator("abc-123_:.")).To(Succeed())
		})
		g.It("rejects disallowed chars", func() {
			Expect(AlphaNumericPunctuationValidator("abc/123")).ToNot(Succeed())
		})
	})

	g.Describe("PrintableValidator", func() {
		g.It("accepts printable unicode", func() {
			Expect(PrintableValidator("abc-123_:.äöüß€")).To(Succeed())
		})
		g.It("rejects non-printable chars", func() {
			Expect(PrintableValidator("abc\x00def")).ToNot(Succeed())
		})
	})

	g.Describe("Base64CharactersValidator", func() {
		g.It("accepts base64", func() {
			Expect(Base64CharactersValidator("YWJjMTIz+/=")).To(Succeed())
		})
		g.It("rejects non-base64", func() {
			Expect(Base64CharactersValidator("abc*123")).ToNot(Succeed())
		})
	})

	g.Describe("BoolValidator", func() {
		g.It("accepts true/false", func() {
			Expect(BoolValidator("true")).To(Succeed())
			Expect(BoolValidator("false")).To(Succeed())
		})
		g.It("rejects non-bool", func() {
			Expect(BoolValidator("yes")).ToNot(Succeed())
		})
	})

	g.Describe("RegExValidator", func() {
		g.It("accepts matching regex", func() {
			v := RegExValidator(regexp.MustCompile(`^foo\d+$`))
			Expect(v("foo123")).To(Succeed())
		})
		g.It("rejects non-matching", func() {
			v := RegExValidator(regexp.MustCompile(`^foo\d+$`))
			Expect(v("bar123")).ToNot(Succeed())
		})
	})

	g.Describe("MaxLengthValidator", func() {
		g.It("accepts within max", func() {
			v := MaxLengthValidator(5)
			Expect(v("12345")).To(Succeed())
		})
		g.It("rejects over max", func() {
			v := MaxLengthValidator(3)
			Expect(v("abcd")).ToNot(Succeed())
		})
	})

	g.Describe("PredefinedValuesValidator", func() {
		g.It("accepts predefined", func() {
			v := PredefinedValuesValidator("foo", "bar")
			Expect(v("foo")).To(Succeed())
		})
		g.It("rejects not predefined", func() {
			v := PredefinedValuesValidator("foo", "bar")
			Expect(v("baz")).ToNot(Succeed())
		})
	})

	g.Describe("IntValidator", func() {
		g.It("accepts in range", func() {
			v := IntValidator(1, 10)
			Expect(v("5")).To(Succeed())
		})
		g.It("rejects out of range", func() {
			v := IntValidator(1, 10)
			Expect(v("11")).ToNot(Succeed())
		})
		g.It("rejects non-int", func() {
			v := IntValidator(1, 10)
			Expect(v("abc")).ToNot(Succeed())
		})
	})

	g.Describe("URLValidator", func() {
		g.It("accepts allowed scheme", func() {
			v := URLValidator("https", "http")
			Expect(v("https://example.com")).To(Succeed())
		})
		g.It("rejects not allowed scheme", func() {
			v := URLValidator("https")
			Expect(v("ftp://example.com")).ToNot(Succeed())
		})
		g.It("rejects invalid url", func() {
			v := URLValidator("https")
			Expect(v("https://%zz")).ToNot(Succeed())
		})
	})

	g.Describe("NoTrailingWhitespaceValidator", func() {
		g.It("accepts no trailing whitespace", func() {
			Expect(NoTrailingWhitespaceValidator("foo")).To(Succeed())
		})
		g.It("rejects trailing whitespace", func() {
			Expect(NoTrailingWhitespaceValidator("foo ")).ToNot(Succeed())
		})
	})

	g.Describe("NoTrailingNewlineValidator", func() {
		g.It("accepts no trailing newline", func() {
			Expect(NoTrailingNewlineValidator("foo")).To(Succeed())
		})
		g.It("rejects trailing newline", func() {
			Expect(NoTrailingNewlineValidator("foo\n")).ToNot(Succeed())
		})
	})

	g.Describe("PEMValidator", func() {
		g.It("accepts valid PEM", func() {
			block := &pem.Block{Type: "CERTIFICATE", Bytes: []byte("test")}
			pemStr := string(pem.EncodeToMemory(block))
			Expect(PEMValidator(pemStr)).To(Succeed())
		})

		g.It("rejects invalid PEM", func() {
			Expect(PEMValidator("not a pem")).ToNot(Succeed())
		})
	})

	g.Describe("CACertValidator", func() {
		g.It("accepts valid CA PEM", func() {
			cacert, err := generateCACert()
			Expect(err).ToNot(HaveOccurred())
			Expect(CACertValidator(cacert)).To(Succeed())
		})

		g.It("rejects invalid PEM", func() {
			Expect(CACertValidator("not a pem")).ToNot(Succeed())
		})
	})

	g.Describe("ExpectedValueValidator", func() {
		g.It("accepts expected value", func() {
			Expect(ExpectedValueValidator("foo")("foo")).To(Succeed())
		})

		g.It("rejects unexpected value", func() {
			Expect(ExpectedValueValidator("foo")("bar")).ToNot(Succeed())
		})
	})
})

func generateCACert() (string, error) {
	ca := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               remoteaccesscertificates.CreateSubject(""),
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
	cacert := pem.EncodeToMemory(&pem.Block{Type: pkiutil.CertificateBlockType, Bytes: crtBytes})

	return string(cacert), nil
}
