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
	"github.com/gardener/controller-manager-library/pkg/logger"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/util/flowcontrol"

	"github.com/gardener/external-dns-management/pkg/dns"
	"github.com/gardener/external-dns-management/pkg/dns/provider"
	dnserrors "github.com/gardener/external-dns-management/pkg/dns/provider/errors"
)

// fakeChangeRecordsAPI is a controllable fake of changeRecordsAPI.
type fakeChangeRecordsAPI struct {
	changeFn    func(*route53.ChangeResourceRecordSetsInput) (*route53.ChangeResourceRecordSetsOutput, error)
	changeCalls []*route53.ChangeResourceRecordSetsInput
}

func (f *fakeChangeRecordsAPI) ChangeResourceRecordSets(_ context.Context, params *route53.ChangeResourceRecordSetsInput, _ ...func(*route53.Options)) (*route53.ChangeResourceRecordSetsOutput, error) {
	f.changeCalls = append(f.changeCalls, params)
	if f.changeFn == nil {
		return &route53.ChangeResourceRecordSetsOutput{}, nil
	}
	return f.changeFn(params)
}

func (f *fakeChangeRecordsAPI) ListResourceRecordSets(_ context.Context, _ *route53.ListResourceRecordSetsInput, _ ...func(*route53.Options)) (*route53.ListResourceRecordSetsOutput, error) {
	return &route53.ListResourceRecordSetsOutput{}, nil
}

// recordingDoneHandler records the terminal outcome reported for a change.
type recordingDoneHandler struct {
	succeeded bool
	failed    bool
	err       error
}

func (h *recordingDoneHandler) SetInvalid(_ error) {}
func (h *recordingDoneHandler) Throttled()         {}
func (h *recordingDoneHandler) Failed(err error)   { h.failed = true; h.err = err }
func (h *recordingDoneHandler) Succeeded()         { h.succeeded = true }

// upsertChange builds an A-record upsert change with the given name/value and done handler.
func upsertChange(name, value string, done provider.DoneHandler) *Change {
	rrs := &route53types.ResourceRecordSet{
		Name:            aws.String(name),
		Type:            route53types.RRTypeA,
		TTL:             aws.Int64(60),
		ResourceRecords: []route53types.ResourceRecord{{Value: aws.String(value)}},
	}
	return &Change{
		Change: &route53types.Change{Action: route53types.ChangeActionUpsert, ResourceRecordSet: rrs},
		Done:   done,
	}
}

func newTestExecution(api changeRecordsAPI) *Execution {
	zone := provider.NewDNSHostedZone(TYPE_CODE, "Z1", "example.org", "/hostedzone/Z1", false)
	return &Execution{
		LogContext:  logger.New(),
		metrics:     &provider.NullMetrics{},
		r53:         api,
		rateLimiter: flowcontrol.NewFakeAlwaysRateLimiter(),
		zone:        zone,
		changes:     map[dns.DNSSetName][]*Change{},
		batchSize:   16,
	}
}

var _ = Describe("Execution submitChanges", func() {
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
	})

	It("bisects a rejected batch so only the bad change fails", func() {
		fake := &fakeChangeRecordsAPI{
			changeFn: func(params *route53.ChangeResourceRecordSetsInput) (*route53.ChangeResourceRecordSetsOutput, error) {
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
		goodDone := &recordingDoneHandler{}
		good2Done := &recordingDoneHandler{}
		badDone := &recordingDoneHandler{}
		exec := newTestExecution(fake)
		exec.changes[dns.DNSSetName{DNSName: "good.example.org"}] = []*Change{upsertChange("good.example.org", "1.2.3.4", goodDone)}
		exec.changes[dns.DNSSetName{DNSName: "good2.example.org"}] = []*Change{upsertChange("good2.example.org", "1.2.3.4", good2Done)}
		exec.changes[dns.DNSSetName{DNSName: "bad.example.org"}] = []*Change{upsertChange("bad.example.org", "bad", badDone)}

		err := exec.submitChanges(ctx)
		Expect(err).To(MatchError(ContainSubstring("1 changes failed")))

		// The valid change was applied and reported success; only the bad one failed.
		Expect(goodDone.succeeded).To(BeTrue())
		Expect(goodDone.failed).To(BeFalse())
		Expect(good2Done.succeeded).To(BeTrue())
		Expect(good2Done.failed).To(BeFalse())
		Expect(badDone.failed).To(BeTrue())
		Expect(badDone.succeeded).To(BeFalse())
	})

	It("does not bisect a fully valid multi-change batch", func() {
		fake := &fakeChangeRecordsAPI{}
		d1 := &recordingDoneHandler{}
		d2 := &recordingDoneHandler{}
		exec := newTestExecution(fake)
		exec.changes[dns.DNSSetName{DNSName: "a.example.org"}] = []*Change{upsertChange("a.example.org", "1.2.3.4", d1)}
		exec.changes[dns.DNSSetName{DNSName: "b.example.org"}] = []*Change{upsertChange("b.example.org", "1.2.3.5", d2)}

		Expect(exec.submitChanges(ctx)).To(Succeed())
		// One batch call, no splitting on the happy path.
		Expect(fake.changeCalls).To(HaveLen(1))
		Expect(fake.changeCalls[0].ChangeBatch.Changes).To(HaveLen(2))
		Expect(d1.succeeded).To(BeTrue())
		Expect(d2.succeeded).To(BeTrue())
	})

	It("does not bisect a throttled batch", func() {
		fake := &fakeChangeRecordsAPI{
			changeFn: func(_ *route53.ChangeResourceRecordSetsInput) (*route53.ChangeResourceRecordSetsOutput, error) {
				return nil, &smithy.GenericAPIError{Code: "Throttling", Message: "Rate exceeded"}
			},
		}
		d1 := &recordingDoneHandler{}
		d2 := &recordingDoneHandler{}
		exec := newTestExecution(fake)
		exec.changes[dns.DNSSetName{DNSName: "a.example.org"}] = []*Change{upsertChange("a.example.org", "1.2.3.4", d1)}
		exec.changes[dns.DNSSetName{DNSName: "b.example.org"}] = []*Change{upsertChange("b.example.org", "1.2.3.5", d2)}

		err := exec.submitChanges(ctx)
		Expect(err).To(HaveOccurred())
		// Both changes were throttled, so this surfaces as an "all changes failed"
		// error wrapping the throttling error; neither is bisected.
		Expect(dnserrors.IsAllChangesFailedError(err)).To(BeTrue())
		Expect(err).To(MatchError(ContainSubstring("Throttling")))
		// The batch is retried as a whole, not bisected: exactly one attempt.
		Expect(fake.changeCalls).To(HaveLen(1))
	})
})
