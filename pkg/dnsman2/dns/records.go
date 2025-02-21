// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dns

import (
	"bytes"
	"fmt"
	"strings"
)

// RecordType represents the type of record set.
type RecordType string

const (
	TypeNS    RecordType = "NS"
	TypeTXT   RecordType = "TXT"
	TypeCNAME RecordType = "CNAME"
	TypeA     RecordType = "A"
	TypeAAAA  RecordType = "AAAA"

	TypeAWS_ALIAS_A    RecordType = "ALIAS"      // provider specific alias for CNAME record (AWS alias target A)
	TypeAWS_ALIAS_AAAA RecordType = "ALIAS_AAAA" // provider specific alias for CNAME record (AWS alias target AAAA)
)

////////////////////////////////////////////////////////////////////////////////
// Record Sets
////////////////////////////////////////////////////////////////////////////////

type RecordSets map[RecordType]*RecordSet

func (rss RecordSets) Clone() RecordSets {
	clone := RecordSets{}
	for rk, rv := range rss {
		clone[rk] = rv.Clone()
	}
	return clone
}

func (rss RecordSets) AddRecord(rtype RecordType, host string, ttl int64) {
	rs := rss[rtype]
	if rs == nil {
		rs = NewRecordSet(rtype, ttl, nil)
		rss[rtype] = rs
	}
	rs.Records = append(rs.Records, &Record{Value: host})
}

type Record struct {
	Value string
}

func (r *Record) Clone() *Record {
	return &Record{r.Value}
}

type RecordSet struct {
	Type      RecordType
	TTL       int64
	IgnoreTTL bool
	Records   []*Record
}

func NewRecordSet(rtype RecordType, ttl int64, records []*Record) *RecordSet {
	return &RecordSet{Type: rtype, TTL: ttl, Records: records}
}

func (rs *RecordSet) Clone() *RecordSet {
	if rs == nil {
		return nil
	}

	set := &RecordSet{Type: rs.Type, TTL: rs.TTL, IgnoreTTL: rs.IgnoreTTL}
	for _, r := range rs.Records {
		set.Records = append(set.Records, r.Clone())
	}
	return set
}

func (rs *RecordSet) Length() int {
	if rs == nil {
		return 0
	}
	return len(rs.Records)
}

func (rs *RecordSet) Add(records ...*Record) *RecordSet {
	for _, r := range records {
		rs.Records = append(rs.Records, r)
	}
	return rs
}

func (rs *RecordSet) RecordString() string {
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

func (rs *RecordSet) Match(set *RecordSet) bool {
	if len(rs.Records) != len(set.Records) {
		return false
	}

	if rs.Type != TypeAWS_ALIAS_A && rs.Type != TypeAWS_ALIAS_AAAA {
		// ignore TTL for alias records
		if !rs.IgnoreTTL && !set.IgnoreTTL && rs.TTL != set.TTL {
			return false
		}
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
	return true
}

func (rs *RecordSet) GetAttr(name string) string {
	if rs.Type == TypeTXT {
		prefix := newAttrKeyPrefix(name)
		for _, r := range rs.Records {
			if strings.HasPrefix(r.Value, prefix) {
				return r.Value[len(prefix) : len(r.Value)-1]
			}
		}
	}
	return ""
}

func (rs *RecordSet) SetAttr(name string, value string) {
	prefix := newAttrKeyPrefix(name)
	for _, r := range rs.Records {
		if strings.HasPrefix(r.Value, prefix) {
			r.Value = newAttrValue(name, value)
			return
		}
	}
	r := newAttrRecord(name, value)
	rs.Records = append(rs.Records, r)
}

func (rs *RecordSet) DeleteAttr(name string) {
	prefix := newAttrKeyPrefix(name)
	for i, r := range rs.Records {
		if strings.HasPrefix(r.Value, prefix) {
			rs.Records = append(rs.Records[:i], rs.Records[i+1:]...)
			return
		}
	}
}

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

func newAttrKeyPrefix(name string) string {
	return fmt.Sprintf("\"%s=", name)
}

func newAttrValue(name, value string) string {
	return fmt.Sprintf("%s%s\"", newAttrKeyPrefix(name), value)
}

func newAttrRecord(name, value string) *Record {
	return &Record{Value: newAttrValue(name, value)}
}

func newAttrRecordSet(rtype RecordType, name, value string) *RecordSet {
	records := []*Record{newAttrRecord(name, value)}
	return &RecordSet{Type: rtype, TTL: 600, IgnoreTTL: false, Records: records}
}
