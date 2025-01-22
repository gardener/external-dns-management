// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dns

import (
	"bytes"
	"fmt"
	"strings"
)

const (
	RS_ALIAS_A    = "ALIAS"      // provider specific alias for CNAME record (AWS alias target A)
	RS_ALIAS_AAAA = "ALIAS_AAAA" // provider specific alias for CNAME record (AWS alias target AAAA)
)

const (
	RS_TXT   = "TXT"
	RS_CNAME = "CNAME"
	RS_A     = "A"
	RS_AAAA  = "AAAA"
)

const RS_NS = "NS"

////////////////////////////////////////////////////////////////////////////////
// Record Sets
////////////////////////////////////////////////////////////////////////////////

type (
	RecordSets map[string]*RecordSet
	Records    []*Record
)

func (rss RecordSets) Clone() RecordSets {
	clone := RecordSets{}
	for rk, rv := range rss {
		clone[rk] = rv.Clone()
	}
	return clone
}

func (rss RecordSets) AddRecord(ty string, host string, ttl int64) {
	rs := rss[ty]
	if rs == nil {
		rs = NewRecordSet(ty, ttl, nil)
		rss[ty] = rs
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
	Type      string
	TTL       int64
	IgnoreTTL bool
	Records   Records
}

func NewRecordSet(rtype string, ttl int64, records []*Record) *RecordSet {
	return &RecordSet{Type: rtype, TTL: ttl, Records: records}
}

func (rs *RecordSet) Clone() *RecordSet {
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

	if rs.Type != RS_ALIAS_A && rs.Type != RS_ALIAS_AAAA {
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
	if rs.Type == RS_TXT {
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

func (rs *RecordSet) DiffTo(set *RecordSet) (new Records, update Records, delete Records) {
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

func newAttrRecordSet(ty string, name, value string) *RecordSet {
	records := []*Record{newAttrRecord(name, value)}
	return &RecordSet{Type: ty, TTL: 600, IgnoreTTL: false, Records: records}
}
