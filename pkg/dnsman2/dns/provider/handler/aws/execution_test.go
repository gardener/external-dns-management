// SPDX-FileCopyrightText: Contributors to the Gardener project
//
// SPDX-License-Identifier: Apache-2.0

package aws

import (
	"context"
	"errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"
	"github.com/aws/smithy-go"
	"github.com/go-logr/logr"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/util/flowcontrol"

	"github.com/gardener/external-dns-management/pkg/dnsman2/dns"
	dnserrors "github.com/gardener/external-dns-management/pkg/dnsman2/dns/provider/errors"
)

// upsertWrappedChange builds an A-record upsert change with the given name/value.
func upsertWrappedChange(name, value string) *wrappedChange {
	rrs := &route53types.ResourceRecordSet{
		Name:            aws.String(name),
		Type:            route53types.RRTypeA,
		TTL:             aws.Int64(60),
		ResourceRecords: []route53types.ResourceRecord{{Value: aws.String(value)}},
	}
	return &wrappedChange{
		Change: &route53types.Change{Action: route53types.ChangeActionUpsert, ResourceRecordSet: rrs},
	}
}

func newTestExecution(r53 route53API) *execution {
	return &execution{
		log:         logr.Discard(),
		r53:         r53,
		rateLimiter: flowcontrol.NewFakeAlwaysRateLimiter(),
		zoneID:      dns.NewZoneID(ProviderType, "Z1"),
		batchSize:   16,
	}
}

var _ = Describe("execution submitChanges", func() {
	var (
		ctx     context.Context
		metrics = noopMetrics{}
	)

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("bisects a rejected batch so only the bad change fails", func() {
		fake := &fakeRoute53{
			changeResourceRecordsFn: func(_ context.Context, params *route53.ChangeResourceRecordSetsInput) (*route53.ChangeResourceRecordSetsOutput, error) {
				for _, c := range params.ChangeBatch.Changes {
					for _, rr := range c.ResourceRecordSet.ResourceRecords {
						if aws.ToString(rr.Value) == "bad" {
							return nil, errors.New("server error")
						}
					}
				}
				return &route53.ChangeResourceRecordSetsOutput{}, nil
			},
		}
		exec := newTestExecution(fake)
		exec.changes = []*wrappedChange{
			upsertWrappedChange("good.example.org.", "1.2.3.4"),
			upsertWrappedChange("good2.example.org.", "1.2.3.4"),
			upsertWrappedChange("bad.example.org.", "bad"),
		}

		err := exec.submitChanges(ctx, metrics)
		Expect(err).To(MatchError(ContainSubstring("1 changes failed")))
	})

	It("does not bisect a fully valid multi-change batch", func() {
		fake := &fakeRoute53{}
		exec := newTestExecution(fake)
		exec.changes = []*wrappedChange{
			upsertWrappedChange("a.example.org.", "1.2.3.4"),
			upsertWrappedChange("b.example.org.", "1.2.3.5"),
		}

		Expect(exec.submitChanges(ctx, metrics)).To(Succeed())
		// One batch call, no splitting on the happy path.
		Expect(fake.changeCalls).To(HaveLen(1))
		Expect(fake.changeCalls[0].ChangeBatch.Changes).To(HaveLen(2))
	})

	It("does not bisect a throttled batch", func() {
		fake := &fakeRoute53{
			changeResourceRecordsFn: func(_ context.Context, _ *route53.ChangeResourceRecordSetsInput) (*route53.ChangeResourceRecordSetsOutput, error) {
				return nil, &smithy.GenericAPIError{Code: "Throttling", Message: "Rate exceeded"}
			},
		}
		exec := newTestExecution(fake)
		exec.changes = []*wrappedChange{
			upsertWrappedChange("a.example.org.", "1.2.3.4"),
			upsertWrappedChange("b.example.org.", "1.2.3.5"),
		}

		err := exec.submitChanges(ctx, metrics)
		Expect(err).To(HaveOccurred())
		// Every batch throttled, so the error is wrapped as a throttling error.
		Expect(dnserrors.IsThrottlingError(err)).To(BeTrue())
		// The batch is retried as a whole, not bisected: exactly one attempt.
		Expect(fake.changeCalls).To(HaveLen(1))
	})

	It("stops bisecting and does not blame a leaf when throttling surfaces mid-bisection", func() {
		// The first (whole-batch) call fails with a non-throttling error, which
		// forces bisection. As soon as bisection issues its first sub-batch call,
		// the API starts throttling. The bisection must then stop splitting and
		// fail the whole sub-batch rather than isolating a valid change as "bad".
		call := 0
		fake := &fakeRoute53{
			changeResourceRecordsFn: func(_ context.Context, _ *route53.ChangeResourceRecordSetsInput) (*route53.ChangeResourceRecordSetsOutput, error) {
				call++
				if call == 1 {
					return nil, errors.New("server error")
				}
				return nil, &smithy.GenericAPIError{Code: "Throttling", Message: "Rate exceeded"}
			},
		}
		exec := newTestExecution(fake)
		exec.changes = []*wrappedChange{
			upsertWrappedChange("a.example.org.", "1.2.3.4"),
			upsertWrappedChange("b.example.org.", "1.2.3.5"),
		}

		err := exec.submitChanges(ctx, metrics)
		Expect(err).To(HaveOccurred())
		// The batch's final error is a throttle, so it is classified as throttling.
		Expect(dnserrors.IsThrottlingError(err)).To(BeTrue())
		// The sub-batch is not split down to single changes once throttling hits:
		// one whole-batch call plus one throttled sub-batch call, no leaf calls.
		Expect(fake.changeCalls).To(HaveLen(2))
	})
})
