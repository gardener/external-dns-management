/*
 * Copyright 2021 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *      http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 *
 */

// This code is based on the code of the Pagination type from https://github.com/aws/aws-sdk-go/blob/main/aws/request/request_pagination.go
// As it was not possible to reuse it directly, the code here is mostly copied from there.

package aws

import (
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/aws/awsutil"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/route53"
)

// orgListResourceRecordSetsPages performs pagination using the AWS SDK directly
func (h *Handler) orgListResourceRecordSetsPages(inp *route53.ListResourceRecordSetsInput, fn func(*route53.ListResourceRecordSetsOutput, bool) bool) error {
	h.config.RateLimiter.Accept()
	return h.r53.ListResourceRecordSetsPages(inp, fn)
}

// listResourceRecordSetsPages performs pagination of result pages itself, as AWS SDK implementation does
// not handle throttling, i.e. it returns an error if throttling occurs.
// This adapted implementation retries several times if a page request fails because of throttling.
func (h *Handler) listResourceRecordSetsPages(input *route53.ListResourceRecordSetsInput, fn func(*route53.ListResourceRecordSetsOutput, bool) bool) error {
	newRequest := func() *request.Request {
		var inCpy *route53.ListResourceRecordSetsInput
		if input != nil {
			tmp := *input
			inCpy = &tmp
		}
		req, _ := h.r53.ListResourceRecordSetsRequest(inCpy)
		req.SetContext(aws.BackgroundContext())
		return req
	}

	nextPageTokens := func(r *request.Request) []interface{} {
		tokens := []interface{}{}
		tokenAdded := false
		for _, outToken := range r.Operation.OutputTokens {
			vs, _ := awsutil.ValuesAtPath(r.Data, outToken)
			if len(vs) == 0 {
				tokens = append(tokens, nil)
				continue
			}
			v := vs[0]

			switch tv := v.(type) {
			case *string:
				if len(aws.StringValue(tv)) == 0 {
					tokens = append(tokens, nil)
					continue
				}
			case string:
				if len(tv) == 0 {
					tokens = append(tokens, nil)
					continue
				}
			}

			tokenAdded = true
			tokens = append(tokens, v)
		}
		if !tokenAdded {
			return nil
		}

		return tokens
	}

	var started bool
	var nextTokens []interface{}
	hasNextPage := func() bool {
		return !started || len(nextTokens) != 0
	}

	throttlingCount := 0
	for hasNextPage() {
		req := newRequest()
		if started {
			for i, intok := range req.Operation.InputTokens {
				awsutil.SetValueAtPath(req.Params, intok, nextTokens[i])
			}
		}

		h.config.RateLimiter.Accept()
		err := req.Send()
		if err != nil {
			if b, ok := err.(awserr.BatchedErrors); ok {
				if throttlingCount < 7 && b.Code() == "Throttling" {
					// do not stop at throttling as all read pages would be lost
					// instead wait some seconds and try again
					time.Sleep(time.Duration(5+throttlingCount) * time.Second)
					throttlingCount++
					continue
				}
			}
			return err
		}
		throttlingCount = 0
		started = true
		nextTokens = nextPageTokens(req)
		if !fn(req.Data.(*route53.ListResourceRecordSetsOutput), !hasNextPage()) {
			break
		}
	}

	return nil
}
