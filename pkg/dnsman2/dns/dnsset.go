// SPDX-FileCopyrightText: 2024 SAP SE or an SAP affiliate company and Gardener contributors
//
// SPDX-License-Identifier: Apache-2.0

package dns

// DNSSet contains record sets for a DNS name. The name is given without
// trailing dot. If the provider requires this dot, it must be removed or added
// whe reading or writing record sets, respectively.
// Supported record set types are:
// - TXT
// - CNAME
// - A
// - AAAA
//
// If multiple CNAME records are given they will be mapped to A records
// by resolving the cnames. This resolution will be updated periodically.
type DNSSet struct {
	Name DNSSetName
	Sets RecordSets
}

// Clone returns a deep copy of the DNSSet.
func (s *DNSSet) Clone() *DNSSet {
	return &DNSSet{
		Name: s.Name,
		Sets: s.Sets.Clone(),
	}
}

// SetRecordSet sets a record set for the given record type.
func (s *DNSSet) SetRecordSet(recordType RecordType, policy *RoutingPolicy, ttl int64, values ...string) {
	records := make([]*Record, len(values))
	for i, r := range values {
		records[i] = &Record{Value: r}
	}
	s.Sets[recordType] = &RecordSet{Type: recordType, TTL: ttl, Records: records, RoutingPolicy: policy}
}

// NewDNSSet creates a new DNSSet.
func NewDNSSet(name DNSSetName) *DNSSet {
	return &DNSSet{Name: name.Normalize(), Sets: RecordSets{}}
}

// Match matches DNSSet equality.
func (s *DNSSet) Match(that *DNSSet) bool {
	return s.match(that, nil)
}

// MatchRecordTypeSubset matches DNSSet equality for given record type subset.
func (s *DNSSet) MatchRecordTypeSubset(that *DNSSet, recordType RecordType) bool {
	return s.match(that, &recordType)
}

func (s *DNSSet) match(that *DNSSet, restrictToRecordType *RecordType) bool {
	if s == that {
		return true
	}
	if s == nil || that == nil {
		return false
	}
	if s.Name != that.Name {
		return false
	}
	if restrictToRecordType != nil {
		rs1, rs2 := s.Sets[*restrictToRecordType], that.Sets[*restrictToRecordType]
		if rs1 != nil && rs2 != nil {
			return rs1.Match(rs2)
		}
		return rs1 == nil && rs2 == nil
	}

	if len(s.Sets) != len(that.Sets) {
		return false
	}
	for k, v := range s.Sets {
		w := that.Sets[k]
		if w == nil {
			return false
		}
		if !v.Match(w) {
			return false
		}
	}
	return true
}

////////////////////////////////////////////////////////////////////////////////

// DNSSets is a map of DNSSetName to DNSSet.
type DNSSets map[DNSSetName]*DNSSet

// AddRecordSet adds a record set to the DNSSets.
func (s DNSSets) AddRecordSet(name DNSSetName, recordSet *RecordSet) {
	name = name.Normalize()
	dnsset := s[name]
	if dnsset == nil {
		dnsset = NewDNSSet(name)
		s[name] = dnsset
	}
	dnsset.Sets[recordSet.Type] = recordSet
	if recordSet.Type == TypeCNAME {
		for i := range recordSet.Records {
			recordSet.Records[i].Value = NormalizeDomainName(recordSet.Records[i].Value)
		}
	}
}

// RemoveRecordSet removes a record set from the DNSSets.
func (s DNSSets) RemoveRecordSet(name DNSSetName, recordType RecordType) {
	name = name.Normalize()
	dnsset := s[name]
	if dnsset != nil {
		delete(dnsset.Sets, recordType)
		if len(dnsset.Sets) == 0 {
			delete(s, name)
		}
	}
}

// Clone returns a deep copy of the DNSSets.
func (s DNSSets) Clone() DNSSets {
	clone := DNSSets{}
	for dk, dv := range s {
		clone[dk] = dv.Clone()
	}
	return clone
}
