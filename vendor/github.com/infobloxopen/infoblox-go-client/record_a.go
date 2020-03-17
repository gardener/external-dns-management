package ibclient

import (
	"fmt"
)

type RecordAOperations interface {
	CreateARecord(recA RecordA) (*RecordA, error)
	GetARecord(recA RecordA) (*[]RecordA, error)
	DeleteARecord(recA RecordA) (string, error)
	UpdateARecord(recA RecordA) (*RecordA, error)
}

type RecordA struct {
	IBBase            `json:"-"`
	Ref               string `json:"_ref,omitempty"`
	Ipv4Addr          string `json:"ipv4addr,omitempty"`
	Name              string `json:"name,omitempty"`
	View              string `json:"view,omitempty"`
	Zone              string `json:"zone,omitempty"`
	Ea                EA     `json:"extattrs,omitempty"`
	NetView           string `json:"omitempty"`
	Cidr              string `json:"omitempty"`
	AddEA             EA     `json:"omitempty"`
	RemoveEA          EA     `json:"omitempty"`
	CreationTime      int    `json:"creation_time,omitempty"`
	Comment           string `json:"comment,omitempty"`
	Creator           string `json:"creator,omitempty"`
	DdnsProtected     bool   `json:"ddns_protected,omitempty"`
	DnsName           string `json:"dns_name,omitempty"`
	ForbidReclamation bool   `json:"forbid_reclamation,omitempty"`
	Reclaimable       bool   `json:"reclaimable,omitempty"`
	Ttl               uint   `json:"ttl,omitempty"`
	UseTtl            bool   `json:"use_ttl,omitempty"`
}

// NewRecordA creates a new A Record type with objectType and returnFields
func NewRecordA(ra RecordA) *RecordA {
	res := ra
	res.objectType = "record:a"

	res.returnFields = []string{"ipv4addr", "name", "view", "zone", "extattrs", "comment", "creation_time",
		"creator", "ddns_protected", "dns_name", "forbid_reclamation", "reclaimable", "ttl", "use_ttl"}
	return &res
}

// CreateARecord takes Name, Ipv4Addr and View of the record to create A Record
// Optional fields: NetView, Ea, Cidr
// Allocates the next available IPv4Addr if IPv4Addr is not passed
func (objMgr *ObjectManager) CreateARecord(recA RecordA) (*RecordA, error) {
	recA.Ea = objMgr.extendEA(recA.Ea)
	recordA := NewRecordA(recA)
	if recA.Ipv4Addr == "" {
		recordA.Ipv4Addr = fmt.Sprintf("func:nextavailableip:%s,%s", recA.Cidr, recA.NetView)
	} else {
		recordA.Ipv4Addr = recA.Ipv4Addr
	}
	ref, err := objMgr.connector.CreateObject(recordA)
	recordA.Ref = ref
	return recordA, err
}

// GetARecord by passing Name, reference ID, IP Address or DNS View
// If no arguments are passed then, all the records are returned
func (objMgr *ObjectManager) GetARecord(recA RecordA) (*[]RecordA, error) {

	var res []RecordA
	recordA := NewRecordA(recA)
	var err error
	if len(recA.Ref) > 0 {
		err = objMgr.connector.GetObject(recordA, recA.Ref, &recordA)
		res = append(res, *recordA)

	} else {
		err = objMgr.connector.GetObject(recordA, "", &res)
		if err != nil || res == nil || len(res) == 0 {
			return nil, err
		}
	}

	return &res, err
}

// DeleteARecord by passing either Reference or Name or IPv4Addr
// If a record with same Ipv4Addr and different name exists, then name and Ipv4Addr has to be passed
// to avoid multiple record deletions
func (objMgr *ObjectManager) DeleteARecord(recA RecordA) (string, error) {
	var res []RecordA
	recordName := NewRecordA(recA)
	if len(recA.Ref) > 0 {
		return objMgr.connector.DeleteObject(recA.Ref)

	} else {
		err := objMgr.connector.GetObject(recordName, "", &res)
		if err != nil || res == nil || len(res) == 0 {
			return "", err

		}
		return objMgr.connector.DeleteObject(res[0].Ref)
	}

}

// UpdateARecord takes Reference ID of the record as an argument
// to update Name, IPv4Addr and EAs of the record
// returns updated Refernce ID
func (objMgr *ObjectManager) UpdateARecord(recA RecordA) (*RecordA, error) {
	var res RecordA
	recordA := RecordA{}
	recordA.returnFields = []string{"name", "ipv4addr", "extattrs"}
	err := objMgr.connector.GetObject(&recordA, recA.Ref, &res)
	if err != nil {
		return nil, err
	}
	res.Name = recA.Name
	res.Ipv4Addr = recA.Ipv4Addr
	for k, v := range recA.AddEA {
		res.Ea[k] = v
	}

	for k := range recA.RemoveEA {
		_, ok := res.Ea[k]
		if ok {
			delete(res.Ea, k)
		}
	}
	reference, err := objMgr.connector.UpdateObject(&res, recA.Ref)
	res.Ref = reference
	return &res, err
}
