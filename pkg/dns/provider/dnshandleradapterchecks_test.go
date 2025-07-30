// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package provider

import (
	"errors"
	"fmt"
	"regexp"

	"github.com/gardener/controller-manager-library/pkg/utils"
	g "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var regionRegex = regexp.MustCompile("^[a-z0-9-]*$") // empty string is explicitly allowed to match the default region

var _ = g.Describe("DNSHandlerAdapterChecks", func() {
	var (
		checks *DNSHandlerAdapterChecks
		props  utils.Properties
	)

	g.BeforeEach(func() {
		checks = NewDNSHandlerAdapterChecks()
		props = utils.Properties{}
	})

	g.Context("AddRequiredProperty and AddOptionalProperty", func() {
		g.It("should add required and optional properties", func() {
			checks.Add(RequiredProperty("foo", "bar"))
			checks.Add(OptionalProperty("baz", "qux"))
			Expect(checks.propertyChecks).To(HaveLen(2))
			Expect(checks.propertyChecks[0].required).To(BeTrue())
			Expect(checks.propertyChecks[1].required).To(BeFalse())
		})
	})

	g.Context("HasPropertyNameOrAlias", func() {
		g.BeforeEach(func() {
			checks.Add(RequiredProperty("foo", "bar"))
			props["foo"] = "value"
		})

		g.It("should return true for property name", func() {
			Expect(checks.HasPropertyNameOrAlias(props, "foo")).To(BeTrue())
		})

		g.It("should return false for unknown property", func() {
			Expect(checks.HasPropertyNameOrAlias(props, "baz")).To(BeFalse())
		})

		g.It("should return true for alias if present", func() {
			props = utils.Properties{"bar": "value"}
			Expect(checks.HasPropertyNameOrAlias(props, "bar")).To(BeTrue())
		})

		g.It("should return false for missing property", func() {
			props = utils.Properties{}
			Expect(checks.HasPropertyNameOrAlias(props, "bar")).To(BeFalse())
		})
	})

	g.Context("ValidateProperties", func() {
		g.BeforeEach(func() {
			checks.Add(RequiredProperty("foo", "bar").Validators(func(val string) error {
				if val == "bad" {
					return errors.New("bad value")
				}
				return nil
			}))
			checks.Add(OptionalProperty("baz"))
		})

		g.It("should succeed for valid required property", func() {
			props["foo"] = "good"
			Expect(checks.ValidateProperties("test", props)).To(Succeed())
		})

		g.It("should fail if required property is missing", func() {
			Expect(checks.ValidateProperties("test", props)).ToNot(Succeed())
		})

		g.It("should fail if required property is empty", func() {
			props["foo"] = ""
			Expect(checks.ValidateProperties("test", props)).ToNot(Succeed())
		})

		g.It("should fail if validator fails", func() {
			props["foo"] = "bad"
			Expect(checks.ValidateProperties("test", props)).ToNot(Succeed())
		})

		g.It("should ignore missing optional property", func() {
			props["foo"] = "good"
			Expect(checks.ValidateProperties("test", props)).To(Succeed())
		})

		g.It("should fail for unknown property", func() {
			props["foo"] = "good"
			props["unknown"] = "val"
			Expect(checks.ValidateProperties("test", props)).ToNot(Succeed())
		})

		g.It("should detect duplicate keys via alias", func() {
			props["foo"] = "good"
			props["bar"] = "other"
			Expect(checks.ValidateProperties("test", props)).ToNot(Succeed())
		})

		g.It("should detect duplicate keys via alias, but ignore them if it is the same value", func() {
			props["foo"] = "good"
			props["bar"] = "good"
			Expect(checks.ValidateProperties("test", props)).To(Succeed())
		})

		g.It("should detect duplicate keys via multiple aliases", func() {
			checks.Add(RequiredProperty("foo2", "bar1", "bar2"))
			props["foo"] = "good"
			props["bar1"] = "x1"
			props["bar2"] = "x2"
			Expect(checks.ValidateProperties("test", props)).ToNot(Succeed())
			props["foo2"] = "x"
			props["bar1"] = "x"
			props["bar2"] = "x"
			Expect(checks.ValidateProperties("test", props)).To(Succeed())
		})

		g.It("should allow empty AWS_REGION", func() {
			checks.Add(OptionalProperty("AWS_REGION").
				Validators(NoTrailingWhitespaceValidator, MaxLengthValidator(32), RegExValidator(regionRegex)).
				AllowEmptyValue())
			props["foo"] = "good"
			props["AWS_REGION"] = ""
			Expect(checks.ValidateProperties("test", props)).To(Succeed())
		})

		g.It("should not allow optional empty property", func() {
			checks.Add(OptionalProperty("optprop").
				Validators(NoTrailingWhitespaceValidator, MaxLengthValidator(32), RegExValidator(regionRegex)))
			props["foo"] = "good"
			props["optprop"] = ""
			Expect(checks.ValidateProperties("test", props)).ToNot(Succeed())
		})

		g.It("should hide value if validator reports message with value", func() {
			checks.Add(OptionalProperty("secret").
				Validators(func(value string) error {
					return fmt.Errorf("invalid secret: %s", value)
				}).HideValue())
			props["foo"] = "good"
			props["secret"] = "topsecret"
			err := checks.ValidateProperties("test", props)
			Expect(err).ToNot(Succeed())
			Expect(err.Error()).To(ContainSubstring("invalid secret: (hidden)"))
		})

		g.Context("RequiredIfUnset", func() {
			g.It("should require a property if the depending property is not set", func() {
				checks.Add(OptionalProperty("optprop").
					RequiredIfUnset([]string{"baz"}))
				props["foo"] = "good"
				err := checks.ValidateProperties("test", props)
				Expect(err).ToNot(Succeed())
				Expect(err.Error()).To(ContainSubstring("property \"optprop\" is required if property \"baz\" is not set"))
			})

			g.It("should require a property if the depending property is empty", func() {
				checks.Add(OptionalProperty("optprop").
					RequiredIfUnset([]string{"baz"}))
				props["foo"] = "good"
				props["baz"] = ""
				err := checks.ValidateProperties("test", props)
				Expect(err).ToNot(Succeed())
				Expect(err.Error()).To(ContainSubstring("property \"optprop\" is required if property \"baz\" is not set"))
			})

			g.It("should require a property if any depending property is not set", func() {
				checks.Add(OptionalProperty("optprop").
					RequiredIfUnset([]string{"baz", "qux"}))
				props["foo"] = "good"
				props["baz"] = "good"
				err := checks.ValidateProperties("test", props)
				Expect(err).ToNot(Succeed())
				Expect(err.Error()).To(ContainSubstring("property \"optprop\" is required if property \"qux\" is not set"))
			})

			g.It("should not require a property if the depending property is set", func() {
				checks.Add(OptionalProperty("optprop").
					RequiredIfUnset([]string{"baz"}))
				props["foo"] = "good"
				props["baz"] = "good"
				Expect(checks.ValidateProperties("test", props)).To(Succeed())
			})
		})
	})
})
