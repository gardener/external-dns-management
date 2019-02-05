/*
 * Copyright 2019 SAP SE or an SAP affiliate company. All rights reserved. This file is licensed under the Apache Software License, v. 2 except as noted otherwise in the LICENSE file
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

package dns

import (
	"fmt"
	"strings"
)

const RS_META = "META"
const RS_TXT = "TXT"
const RS_CNAME = "CNAME"
const RS_A = "A"

const RS_NS = "NS"

////////////////////////////////////////////////////////////////////////////////
// Record Sets
////////////////////////////////////////////////////////////////////////////////

type RecordSets map[string]*RecordSet
type Records []*Record

type Record struct {
	Value string
}

func (this *Record) Clone() *Record {
	return &Record{this.Value}
}

type RecordSet struct {
	Type    string
	TTL     int64
	Records Records
}

func NewRecordSet(rtype string, ttl int64, records []*Record) *RecordSet {
	if records == nil {
		records = Records{}
	}
	return &RecordSet{Type: rtype, TTL: ttl, Records: records}
}

func (this *RecordSet) Clone() *RecordSet {
	set := &RecordSet{this.Type, this.TTL, nil}
	for _, r := range this.Records {
		set.Records = append(set.Records, r.Clone())
	}
	return set
}

func (this *RecordSet) Length() int {
	if this == nil {
		return 0
	}
	return len(this.Records)
}

func (this *RecordSet) Add(records ...*Record) *RecordSet {
	for _, r := range records {
		this.Records = append(this.Records, r)
	}
	return this
}

func (this *RecordSet) RecordString() string {
	line := ""
	sep := ""
	for _, r := range this.Records {
		line = fmt.Sprintf("%s%s%s", line, sep, r.Value)
		sep = ", "
	}
	if line == "" {
		return "no records"
	}
	return "[" + line + "]"
}

func (this *RecordSet) Match(set *RecordSet) bool {
	if len(this.Records) != len(set.Records) {
		return false
	}

	if this.TTL != set.TTL {
		return false
	}

	for _, r := range this.Records {
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

func (this *RecordSet) GetAttr(name string) string {
	if this.Type == RS_TXT || this.Type == RS_META {
		prefix := newMetaKeyPrefix(name)
		for _, r := range this.Records {
			if strings.HasPrefix(r.Value, prefix) {
				return r.Value[len(prefix) : len(r.Value)-1]
			}
		}
	}
	return ""
}

func (this *RecordSet) SetAttr(name string, value string) {
	prefix := newMetaKeyPrefix(name)
	for _, r := range this.Records {
		if strings.HasPrefix(r.Value, prefix) {
			r.Value = newMetaValue(name, value)
			return
		}
	}
	r := newMetaRecord(name, value)
	this.Records = append(this.Records, r)
}

func (this *RecordSet) DiffTo(set *RecordSet) (new Records, update Records, delete Records) {
nextOwn:
	for _, r := range this.Records {
		for _, d := range set.Records {
			if d.Value == r.Value {
				if this.TTL != set.TTL {
					update = append(update, r)
				}
				continue nextOwn
			}
		}
		new = append(new, r)
	}
nextForeign:
	for _, d := range set.Records {
		for _, r := range this.Records {
			if d.Value == r.Value {
				continue nextForeign
			}
		}
		delete = append(delete, d)
	}
	return
}

func newMetaKeyPrefix(name string) string {
	return fmt.Sprintf("\"%s=", name)
}

func newMetaValue(name, value string) string {
	return fmt.Sprintf("%s%s\"", newMetaKeyPrefix(name), value)
}

func newMetaRecord(name, value string) *Record {
	return &Record{Value: newMetaValue(name, value)}
}

func newMetaRecordSet(name, value string) *RecordSet {
	records := []*Record{newMetaRecord(name, value)}
	return &RecordSet{RS_META, 600, records}
}
