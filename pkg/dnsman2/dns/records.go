// SPDX-FileCopyrightText: SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dns

import (
	"bytes"
	"fmt"
	"reflect"
)

// RecordType represents the type of record set.
type RecordType string

const (
	// TypeNS represents a DNS NS record.
	TypeNS RecordType = "NS"
	// TypeTXT represents a DNS TXT record.
	TypeTXT RecordType = "TXT"
	// TypeCNAME represents a DNS CNAME record.
	TypeCNAME RecordType = "CNAME"
	// TypeA represents a DNS A record.
	TypeA RecordType = "A"
	// TypeAAAA represents a DNS AAAA record.
	TypeAAAA RecordType = "AAAA"

	// TypeAWS_ALIAS_A represents a provider-specific alias for CNAME record (AWS alias target A).
	TypeAWS_ALIAS_A RecordType = "ALIAS"
	// TypeAWS_ALIAS_AAAA represents a provider-specific alias for CNAME record (AWS alias target AAAA).
	TypeAWS_ALIAS_AAAA RecordType = "ALIAS_AAAA"
)

////////////////////////////////////////////////////////////////////////////////
// Record Sets
////////////////////////////////////////////////////////////////////////////////

// RecordSets is a map of RecordType to RecordSet.
type RecordSets map[RecordType]*RecordSet

// Clone returns a deep copy of the RecordSets.
func (rss RecordSets) Clone() RecordSets {
	clone := RecordSets{}
	for rk, rv := range rss {
		clone[rk] = rv.Clone()
	}
	return clone
}

// AddRecord adds a record to the RecordSets for the given type, host, and TTL.
func (rss RecordSets) AddRecord(rtype RecordType, host string, ttl int64) {
	rs := rss[rtype]
	if rs == nil {
		rs = NewRecordSet(rtype, ttl, nil)
		rss[rtype] = rs
	}
	rs.Records = append(rs.Records, &Record{Value: host})
}

// Record represents a single DNS record value.
type Record struct {
	Value string
}

// Clone returns a deep copy of the Record.
func (r *Record) Clone() *Record {
	return &Record{r.Value}
}

// RecordSet represents a set of DNS records of the same type.
type RecordSet struct {
	Type          RecordType
	TTL           int64
	Records       []*Record
	RoutingPolicy *RoutingPolicy
}

// NewRecordSet creates a new RecordSet with the given type, TTL, and records.
func NewRecordSet(rtype RecordType, ttl int64, records []*Record) *RecordSet {
	return &RecordSet{Type: rtype, TTL: ttl, Records: records}
}

// IsTTLIgnored returns true if the TTL is ignored for this RecordSet type.
func (rs *RecordSet) IsTTLIgnored() bool {
	if rs == nil {
		return false
	}
	return rs.Type == TypeAWS_ALIAS_A || rs.Type == TypeAWS_ALIAS_AAAA
}

// Clone returns a deep copy of the RecordSet.
func (rs *RecordSet) Clone() *RecordSet {
	if rs == nil {
		return nil
	}

	set := &RecordSet{Type: rs.Type, TTL: rs.TTL}
	for _, r := range rs.Records {
		set.Records = append(set.Records, r.Clone())
	}
	if rs.RoutingPolicy != nil {
		set.RoutingPolicy = rs.RoutingPolicy.Clone()
	}
	return set
}

// Length returns the number of records in the RecordSet.
func (rs *RecordSet) Length() int {
	if rs == nil {
		return 0
	}
	return len(rs.Records)
}

// Add appends the given records to the RecordSet and returns the updated RecordSet.
func (rs *RecordSet) Add(records ...*Record) *RecordSet {
	rs.Records = append(rs.Records, records...)
	return rs
}

// RecordString returns a string representation of the records in the RecordSet.
func (rs *RecordSet) RecordString() string {
	if rs == nil {
		return "null"
	}
	line := ""
	sep := ""
	for _, r := range rs.Records {
		line = fmt.Sprintf("%s%s%s", line, sep, r.Value)
		sep = ", "
	}
	if line == "" {
		return "no records"
	}
	return "[" + line + "]"
}

// Match checks if the current RecordSet matches the given RecordSet.
func (rs *RecordSet) Match(set *RecordSet) bool {
	if len(rs.Records) != len(set.Records) {
		return false
	}

	if !rs.IsTTLIgnored() && rs.TTL != set.TTL {
		return false
	}

	for _, r := range rs.Records {
		found := false
		for _, t := range set.Records {
			if t.Value == r.Value {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	return reflect.DeepEqual(rs.RoutingPolicy, set.RoutingPolicy)
}

// DiffTo compares the current RecordSet with another RecordSet and returns the records that are new, updated, or deleted.
func (rs *RecordSet) DiffTo(set *RecordSet) (new, update, delete []*Record) {
nextOwn:
	for _, r := range rs.Records {
		for _, d := range set.Records {
			if d.Value == r.Value {
				if rs.TTL != set.TTL {
					update = append(update, r)
				}
				continue nextOwn
			}
		}
		new = append(new, r)
	}
nextForeign:
	for _, d := range set.Records {
		for _, r := range rs.Records {
			if d.Value == r.Value {
				continue nextForeign
			}
		}
		delete = append(delete, d)
	}
	return
}

// String returns a string representation of the RecordSet.
func (rs *RecordSet) String() string {
	if rs == nil {
		return ""
	}
	var buf bytes.Buffer
	for i, rec := range rs.Records {
		if i > 0 {
			buf.WriteString(",")
		}
		buf.WriteString(rec.Value)
	}
	return buf.String()
}
