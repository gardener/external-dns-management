package ibclient

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
)

// Compile-time interface checks
var _ IBObjectManager = new(ObjectManager)

type IBObjectManager interface {
	AllocateIP(netview string, cidr string, ipAddr string, isIPv6 bool, macOrDuid string, name string, comment string, eas EA) (*FixedAddress, error)
	AllocateNetwork(netview string, cidr string, isIPv6 bool, prefixLen uint, comment string, eas EA) (network *Network, err error)
	CreateARecord(netView string, dnsView string, name string, cidr string, ipAddr string, ttl uint32, useTTL bool, comment string, ea EA) (*RecordA, error)
	CreateAAAARecord(netView string, dnsView string, recordName string, cidr string, ipAddr string, useTtl bool, ttl uint32, comment string, eas EA) (*RecordAAAA, error)
	CreateZoneAuth(fqdn string, ea EA) (*ZoneAuth, error)
	CreateCNAMERecord(dnsview string, canonical string, recordname string, useTtl bool, ttl uint32, comment string, eas EA) (*RecordCNAME, error)
	CreateDefaultNetviews(globalNetview string, localNetview string) (globalNetviewRef string, localNetviewRef string, err error)
	CreateEADefinition(eadef EADefinition) (*EADefinition, error)
	CreateHostRecord(enabledns bool, enabledhcp bool, recordName string, netview string, dnsview string, ipv4cidr string, ipv6cidr string, ipv4Addr string, ipv6Addr string, macAddr string, duid string, useTtl bool, ttl uint32, comment string, eas EA, aliases []string) (*HostRecord, error)
	CreateNetwork(netview string, cidr string, isIPv6 bool, comment string, eas EA) (*Network, error)
	CreateNetworkContainer(netview string, cidr string, isIPv6 bool, comment string, eas EA) (*NetworkContainer, error)
	CreateNetworkView(name string, comment string, setEas EA) (*NetworkView, error)
	CreatePTRRecord(networkView string, dnsView string, ptrdname string, recordName string, cidr string, ipAddr string, useTtl bool, ttl uint32, comment string, eas EA) (*RecordPTR, error)
	CreateTXTRecord(recordname string, text string, ttl uint, dnsview string) (*RecordTXT, error)
	CreateZoneDelegated(fqdn string, delegate_to []NameServer) (*ZoneDelegated, error)
	DeleteARecord(ref string) (string, error)
	DeleteAAAARecord(ref string) (string, error)
	DeleteZoneAuth(ref string) (string, error)
	DeleteCNAMERecord(ref string) (string, error)
	DeleteFixedAddress(ref string) (string, error)
	DeleteHostRecord(ref string) (string, error)
	DeleteNetwork(ref string) (string, error)
	DeleteNetworkContainer(ref string) (string, error)
	DeleteNetworkView(ref string) (string, error)
	DeletePTRRecord(ref string) (string, error)
	DeleteTXTRecord(ref string) (string, error)
	DeleteZoneDelegated(ref string) (string, error)
	GetARecordByRef(ref string) (*RecordA, error)
	GetARecord(dnsview string, recordName string, ipAddr string) (*RecordA, error)
	GetAAAARecord(dnsview string, recordName string, ipAddr string) (*RecordAAAA, error)
	GetAAAARecordByRef(ref string) (*RecordAAAA, error)
	GetCNAMERecord(dnsview string, canonical string, recordName string) (*RecordCNAME, error)
	GetCNAMERecordByRef(ref string) (*RecordCNAME, error)
	GetEADefinition(name string) (*EADefinition, error)
	GetFixedAddress(netview string, cidr string, ipAddr string, isIPv6 bool, macOrDuid string) (*FixedAddress, error)
	GetFixedAddressByRef(ref string) (*FixedAddress, error)
	GetHostRecord(netview string, dnsview string, recordName string, ipv4addr string, ipv6addr string) (*HostRecord, error)
	GetHostRecordByRef(ref string) (*HostRecord, error)
	GetIpAddressFromHostRecord(host HostRecord) (string, error)
	GetNetwork(netview string, cidr string, isIPv6 bool, ea EA) (*Network, error)
	GetNetworkByRef(ref string) (*Network, error)
	GetNetworkContainer(netview string, cidr string, isIPv6 bool, eaSearch EA) (*NetworkContainer, error)
	GetNetworkContainerByRef(ref string) (*NetworkContainer, error)
	GetNetworkView(name string) (*NetworkView, error)
	GetNetworkViewByRef(ref string) (*NetworkView, error)
	GetPTRRecord(dnsview string, ptrdname string, recordName string, ipAddr string) (*RecordPTR, error)
	GetPTRRecordByRef(ref string) (*RecordPTR, error)
	GetZoneAuthByRef(ref string) (*ZoneAuth, error)
	GetZoneDelegated(fqdn string) (*ZoneDelegated, error)
	GetCapacityReport(name string) ([]CapacityReport, error)
	GetUpgradeStatus(statusType string) ([]UpgradeStatus, error)
	GetAllMembers() ([]Member, error)
	GetGridInfo() ([]Grid, error)
	GetGridLicense() ([]License, error)
	ReleaseIP(netview string, cidr string, ipAddr string, isIPv6 bool, macAddr string) (string, error)
	UpdateAAAARecord(ref string, netView string, recordName string, cidr string, ipAddr string, useTtl bool, ttl uint32, comment string, setEas EA) (*RecordAAAA, error)
	UpdateCNAMERecord(ref string, canonical string, recordName string, useTtl bool, ttl uint32, comment string, setEas EA) (*RecordCNAME, error)
	UpdateFixedAddress(fixedAddrRef string, netview string, name string, cidr string, ipAddr string, matchclient string, macOrDuid string, comment string, eas EA) (*FixedAddress, error)
	UpdateHostRecord(hostRref string, enabledns bool, enabledhcp bool, name string, netview string, ipv4cidr string, ipv6cidr string, ipv4Addr string, ipv6Addr string, macAddress string, duid string, useTtl bool, ttl uint32, comment string, eas EA, aliases []string) (*HostRecord, error)
	UpdateNetwork(ref string, setEas EA, comment string) (*Network, error)
	UpdateNetworkContainer(ref string, setEas EA, comment string) (*NetworkContainer, error)
	UpdateNetworkView(ref string, name string, comment string, setEas EA) (*NetworkView, error)
	UpdatePTRRecord(ref string, netview string, ptrdname string, name string, cidr string, ipAddr string, useTtl bool, ttl uint32, comment string, setEas EA) (*RecordPTR, error)
	UpdateARecord(ref string, name string, ipAddr string, cidr string, netview string, ttl uint32, useTTL bool, comment string, eas EA) (*RecordA, error)
	UpdateZoneDelegated(ref string, delegate_to []NameServer) (*ZoneDelegated, error)
}

type NotFoundError struct {
	msg string
}

func (e *NotFoundError) Error() string {
	return "not found"
}

func NewNotFoundError(msg string) *NotFoundError {
	return &NotFoundError{msg: msg}
}

type ObjectManager struct {
	connector IBConnector
	cmpType   string
	tenantID  string
}

func NewObjectManager(connector IBConnector, cmpType string, tenantID string) IBObjectManager {
	objMgr := &ObjectManager{}

	objMgr.connector = connector
	objMgr.cmpType = cmpType
	objMgr.tenantID = tenantID

	return objMgr
}

func (objMgr *ObjectManager) CreateNetworkView(name string, comment string, setEas EA) (*NetworkView, error) {
	networkView := NewNetworkView(name, comment, setEas, "")

	ref, err := objMgr.connector.CreateObject(networkView)
	networkView.Ref = ref

	return networkView, err
}

func (objMgr *ObjectManager) makeNetworkView(netviewName string) (netviewRef string, err error) {
	var netviewObj *NetworkView
	if netviewObj, err = objMgr.GetNetworkView(netviewName); err != nil {
		return
	}
	if netviewObj == nil {
		if netviewObj, err = objMgr.CreateNetworkView(netviewName, "", nil); err != nil {
			return
		}
	}

	netviewRef = netviewObj.Ref

	return
}

func (objMgr *ObjectManager) CreateDefaultNetviews(globalNetview string, localNetview string) (globalNetviewRef string, localNetviewRef string, err error) {
	if globalNetviewRef, err = objMgr.makeNetworkView(globalNetview); err != nil {
		return
	}

	if localNetviewRef, err = objMgr.makeNetworkView(localNetview); err != nil {
		return
	}

	return
}

func (objMgr *ObjectManager) CreateNetwork(netview string, cidr string, isIPv6 bool, comment string, eas EA) (*Network, error) {
	network := NewNetwork(netview, cidr, isIPv6, comment, eas)

	ref, err := objMgr.connector.CreateObject(network)
	if err != nil {
		return nil, err
	}
	network.Ref = ref

	return network, err
}

func (objMgr *ObjectManager) CreateNetworkContainer(netview string, cidr string, isIPv6 bool, comment string, eas EA) (*NetworkContainer, error) {
	container := NewNetworkContainer(netview, cidr, isIPv6, comment, eas)

	ref, err := objMgr.connector.CreateObject(container)
	if err != nil {
		return nil, err
	}

	container.Ref = ref
	return container, nil
}

func (objMgr *ObjectManager) GetNetworkView(name string) (*NetworkView, error) {
	var res []NetworkView

	netview := NewEmptyNetworkView()
	sf := map[string]string{
		"name": name,
	}
	queryParams := NewQueryParams(false, sf)
	err := objMgr.connector.GetObject(netview, "", queryParams, &res)

	if err != nil {
		return nil, err
	}
	if res == nil || len(res) == 0 {
		return nil, fmt.Errorf("network view '%s' not found", name)
	}

	return &res[0], nil
}

func (objMgr *ObjectManager) GetNetworkViewByRef(ref string) (*NetworkView, error) {
	res := NewEmptyNetworkView()
	queryParams := NewQueryParams(false, nil)
	if err := objMgr.connector.GetObject(res, ref, queryParams, &res); err != nil {
		return nil, err
	}
	if res == nil {
		return nil, fmt.Errorf("network view not found")
	}

	return res, nil
}

func (objMgr *ObjectManager) UpdateNetworkView(ref string, name string, comment string, setEas EA) (*NetworkView, error) {

	nv := NewEmptyNetworkView()

	err := objMgr.connector.GetObject(
		nv, ref, NewQueryParams(false, nil), nv)
	if err != nil {
		return nil, err
	}
	cleanName := strings.TrimSpace(name)
	if cleanName != "" {
		nv.Name = cleanName
	}
    nv.Comment = comment
	nv.Ea = setEas

	updatedRef, err := objMgr.connector.UpdateObject(nv, ref)
	nv.Ref = updatedRef

	return nv, err
}

func BuildNetworkViewFromRef(ref string) *NetworkView {
	// networkview/ZG5zLm5ldHdvcmtfdmlldyQyMw:global_view/false
	r := regexp.MustCompile(`networkview/\w+:([^/]+)/\w+`)
	m := r.FindStringSubmatch(ref)

	if m == nil {
		return nil
	}

	return &NetworkView{
		Ref:  ref,
		Name: m[1],
	}
}

func BuildNetworkFromRef(ref string) (*Network, error) {
	// network/ZG5zLm5ldHdvcmskODkuMC4wLjAvMjQvMjU:89.0.0.0/24/global_view
	r := regexp.MustCompile(`network/\w+:(\d+\.\d+\.\d+\.\d+/\d+)/(.+)`)
	m := r.FindStringSubmatch(ref)

	if m == nil {
		return nil, fmt.Errorf("CIDR format not matched")
	}

	newNet := NewNetwork(m[2], m[1], false, "", nil)
	newNet.Ref = ref
	return newNet, nil
}

func (objMgr *ObjectManager) GetNetwork(netview string, cidr string, isIPv6 bool, ea EA) (*Network, error) {
	if netview != "" && cidr != "" {
		var res []Network

		network := NewNetwork(netview, cidr, isIPv6, "", ea)

		network.Cidr = cidr

		if ea != nil && len(ea) > 0 {
			network.eaSearch = EASearch(ea)
		}

		sf := map[string]string{
			"network_view": netview,
			"network":      cidr,
		}
		queryParams := NewQueryParams(false, sf)
		err := objMgr.connector.GetObject(network, "", queryParams, &res)

		if err != nil {
			return nil, err
		} else if res == nil || len(res) == 0 {
			return nil, NewNotFoundError(
				fmt.Sprintf(
					"Network with cidr: %s in network view: %s is not found.",
					cidr, netview))
		}

		return &res[0], nil
	} else {
		err := fmt.Errorf("both network view and cidr values are required")
		return nil, err
	}
}

func (objMgr *ObjectManager) GetNetworkByRef(ref string) (*Network, error) {
	r := regexp.MustCompile("^ipv6network\\/.+")
	isIPv6 := r.MatchString(ref)

	network := NewNetwork("", "", isIPv6, "", nil)
	err := objMgr.connector.GetObject(network, ref, NewQueryParams(false, nil), network)
	return network, err
}

// TODO normalize IPv4 and IPv6 addresses
func (objMgr *ObjectManager) GetNetworkContainer(netview string, cidr string, isIPv6 bool, eaSearch EA) (*NetworkContainer, error) {
	var res []NetworkContainer

	nc := NewNetworkContainer(netview, cidr, isIPv6, "", nil)
	nc.eaSearch = EASearch(eaSearch)
	sf := map[string]string{
		"network_view": netview,
		"network":      cidr,
	}
	queryParams := NewQueryParams(false, sf)
	err := objMgr.connector.GetObject(nc, "", queryParams, &res)
	if err != nil {
		return nil, err
	} else if res == nil || len(res) == 0 {
		return nil, NewNotFoundError("network container not found")
	}

	return &res[0], nil
}

func (objMgr *ObjectManager) GetNetworkContainerByRef(ref string) (*NetworkContainer, error) {
	nc := NewNetworkContainer("", "", false, "", nil)

	err := objMgr.connector.GetObject(
		nc, ref, NewQueryParams(false, nil), nc)
	if err != nil {
		return nil, err
	}

	return nc, nil
}

func GetIPAddressFromRef(ref string) string {
	// fixedaddress/ZG5zLmJpbmRfY25h:12.0.10.1/external
	r := regexp.MustCompile(`fixedaddress/\w+:(\d+\.\d+\.\d+\.\d+)/.+`)
	m := r.FindStringSubmatch(ref)

	if m != nil {
		return m[1]
	}
	return ""
}

func (objMgr *ObjectManager) AllocateIP(
	netview string,
	cidr string,
	ipAddr string,
	isIPv6 bool,
	macOrDuid string,
	name string,
	comment string,
	eas EA) (*FixedAddress, error) {

	if isIPv6 {
		if len(macOrDuid) == 0 {
			return nil, fmt.Errorf("the DUID field cannot be left empty")
		}
	} else {
		if len(macOrDuid) == 0 {
			macOrDuid = MACADDR_ZERO
		}
	}
	if ipAddr == "" && cidr != "" {
		if netview == "" {
			netview = "default"
		}
		ipAddr = fmt.Sprintf("func:nextavailableip:%s,%s", cidr, netview)
	}
	fixedAddr := NewFixedAddress(
		netview, name, ipAddr, cidr, macOrDuid, "", eas, "", isIPv6, comment)
	ref, err := objMgr.connector.CreateObject(fixedAddr)
	if err != nil {
		return nil, err
	}

	fixedAddr.Ref = ref
	fixedAddr, err = objMgr.GetFixedAddressByRef(ref)

	return fixedAddr, err
}

func (objMgr *ObjectManager) AllocateNetwork(
	netview string,
	cidr string,
	isIPv6 bool,
	prefixLen uint,
	comment string,
	eas EA) (network *Network, err error) {

	network = nil
	cidr = fmt.Sprintf("func:nextavailablenetwork:%s,%s,%d", cidr, netview, prefixLen)
	networkReq := NewNetwork(netview, cidr, isIPv6, comment, eas)

	ref, err := objMgr.connector.CreateObject(networkReq)
	if err == nil {
		if isIPv6 {
			network, err = BuildIPv6NetworkFromRef(ref)
		} else {
			network, err = BuildNetworkFromRef(ref)
		}
	}

	return
}

func (objMgr *ObjectManager) GetFixedAddress(netview string, cidr string, ipAddr string, isIpv6 bool, macOrDuid string) (*FixedAddress, error) {
	var res []FixedAddress

	fixedAddr := NewEmptyFixedAddress(isIpv6)
	sf := map[string]string{
		"network_view": netview,
		"network":      cidr,
	}
	if isIpv6 {
		sf["ipv6addr"] = ipAddr
		if macOrDuid != "" {
			sf["duid"] = macOrDuid
		}
	} else {
		sf["ipv4addr"] = ipAddr
		if macOrDuid != "" {
			sf["mac"] = macOrDuid
		}
	}

	queryParams := NewQueryParams(false, sf)
	err := objMgr.connector.GetObject(fixedAddr, "", queryParams, &res)

	if err != nil || res == nil || len(res) == 0 {
		return nil, err
	}

	return &res[0], nil
}

func (objMgr *ObjectManager) GetFixedAddressByRef(ref string) (*FixedAddress, error) {
	r := regexp.MustCompile("^ipv6fixedaddress/.+")
	isIPv6 := r.MatchString(ref)

	fixedAddr := NewEmptyFixedAddress(isIPv6)
	err := objMgr.connector.GetObject(
		fixedAddr, ref, NewQueryParams(false, nil), &fixedAddr)
	return fixedAddr, err
}

func (objMgr *ObjectManager) DeleteFixedAddress(ref string) (string, error) {
	return objMgr.connector.DeleteObject(ref)
}

// validation  for match_client
func validateMatchClient(value string) bool {
	matchClientList := [5]string{
		"MAC_ADDRESS",
		"CLIENT_ID",
		"RESERVED",
		"CIRCUIT_ID",
		"REMOTE_ID"}

	for _, val := range matchClientList {
		if val == value {
			return true
		}
	}
	return false
}

func (objMgr *ObjectManager) UpdateFixedAddress(
	fixedAddrRef string,
	netview string,
	name string,
	cidr string,
	ipAddr string,
	matchClient string,
	macOrDuid string,
	comment string,
	eas EA) (*FixedAddress, error) {

	r := regexp.MustCompile("^ipv6fixedaddress/.+")
	isIPv6 := r.MatchString(fixedAddrRef)
	if !isIPv6 {
		if !validateMatchClient(matchClient) {
			return nil, fmt.Errorf("wrong value for match_client passed %s \n ", matchClient)
		}
	}
	updateFixedAddr := NewFixedAddress(
		"", name, "", "",
		macOrDuid, matchClient, eas, fixedAddrRef, isIPv6, comment)

	if ipAddr == "" {
		if cidr != "" {
			ipAddress, _, err := net.ParseCIDR(cidr)
			if err != nil {
				return nil, fmt.Errorf("cannot parse CIDR value: %s", err.Error())
			}
			if netview == "" {
				netview = "default"
			}
			if isIPv6 {
				if ipAddress.To4() != nil || ipAddress.To16() == nil {
					return nil, fmt.Errorf("CIDR value must be an IPv6 CIDR, not an IPv4 one")
				}
				updateFixedAddr.IPv6Address = fmt.Sprintf("func:nextavailableip:%s,%s", cidr, netview)
			} else {
				if ipAddress.To4() == nil {
					return nil, fmt.Errorf("CIDR value must be an IPv4 CIDR, not an IPv6 one")
				}
				updateFixedAddr.IPv4Address = fmt.Sprintf("func:nextavailableip:%s,%s", cidr, netview)
			}
		}
	} else {
		ipAddress := net.ParseIP(ipAddr)
		if ipAddress == nil {
			return nil, fmt.Errorf("IP address for the record is not valid")
		}
		if isIPv6 {
			if ipAddress.To4() != nil || ipAddress.To16() == nil {
				return nil, fmt.Errorf("IP address must be an IPv6 address, not an IPv4 one")
			}
			updateFixedAddr.IPv6Address = ipAddr
		} else {
			if ipAddress.To4() == nil {
				return nil, fmt.Errorf("IP address must be an IPv4 address, not an IPv6 one")
			}
			updateFixedAddr.IPv4Address = ipAddr
		}
	}
	refResp, err := objMgr.connector.UpdateObject(updateFixedAddr, fixedAddrRef)

	updateFixedAddr, err = objMgr.GetFixedAddressByRef(refResp)
	if err != nil {
		return nil, err
	}
	return updateFixedAddr, nil
}

func (objMgr *ObjectManager) ReleaseIP(netview string, cidr string, ipAddr string, isIpv6 bool, macOrDuid string) (string, error) {
	fixAddress, _ := objMgr.GetFixedAddress(netview, cidr, ipAddr, isIpv6, macOrDuid)
	if fixAddress == nil {
		return "", nil
	}
	return objMgr.connector.DeleteObject(fixAddress.Ref)
}

func (objMgr *ObjectManager) DeleteNetworkContainer(ref string) (string, error) {
	ncRegExp := regexp.MustCompile("^(ipv6)?networkcontainer\\/.+")
	if !ncRegExp.MatchString(ref) {
		return "", fmt.Errorf("'ref' does not reference a network container")
	}

	return objMgr.connector.DeleteObject(ref)
}

func (objMgr *ObjectManager) DeleteNetwork(ref string) (string, error) {
	return objMgr.connector.DeleteObject(ref)
}

func (objMgr *ObjectManager) DeleteNetworkView(ref string) (string, error) {
	return objMgr.connector.DeleteObject(ref)
}

func (objMgr *ObjectManager) GetEADefinition(name string) (*EADefinition, error) {
	var res []EADefinition

	eadef := NewEADefinition(EADefinition{Name: name})

	sf := map[string]string{
		"name": name,
	}
	queryParams := NewQueryParams(false, sf)
	err := objMgr.connector.GetObject(eadef, "", queryParams, &res)

	if err != nil || res == nil || len(res) == 0 {
		return nil, err
	}

	return &res[0], nil
}

func (objMgr *ObjectManager) CreateEADefinition(eadef EADefinition) (*EADefinition, error) {
	newEadef := NewEADefinition(eadef)

	ref, err := objMgr.connector.CreateObject(newEadef)
	newEadef.Ref = ref

	return newEadef, err
}

func BuildIPv6NetworkFromRef(ref string) (*Network, error) {
	// ipv6network/ZG5zLm5ldHdvcmskODkuMC4wLjAvMjQvMjU:2001%3Adb8%3Aabcd%3A0012%3A%3A0/64/global_view
	r := regexp.MustCompile(`ipv6network/[^:]+:(([^\/]+)\/\d+)\/(.+)`)
	m := r.FindStringSubmatch(ref)

	if m == nil {
		return nil, fmt.Errorf("CIDR format not matched")
	}

	cidr, err := url.QueryUnescape(m[1])
	if err != nil {
		return nil, fmt.Errorf(
			"cannot extract network CIDR information from the reference '%s': %s",
			ref, err.Error())
	}

	if _, _, err = net.ParseCIDR(cidr); err != nil {
		return nil, fmt.Errorf("CIDR format not matched")
	}

	newNet := NewNetwork(m[3], cidr, true, "", nil)
	newNet.Ref = ref

	return newNet, nil
}

func (objMgr *ObjectManager) CreateHostRecord(
	enabledns bool,
	enabledhcp bool,
	recordName string,
	netview string,
	dnsview string,
	ipv4cidr string,
	ipv6cidr string,
	ipv4Addr string,
	ipv6Addr string,
	macAddr string,
	duid string,
	useTtl bool,
	ttl uint32,
	comment string,
	eas EA,
	aliases []string) (*HostRecord, error) {

	if ipv4Addr == "" && ipv4cidr != "" {
		if netview == "" {
			netview = "default"
		}
		ipv4Addr = fmt.Sprintf("func:nextavailableip:%s,%s", ipv4cidr, netview)
	}
	if ipv6Addr == "" && ipv6cidr != "" {
		if netview == "" {
			netview = "default"
		}
		ipv6Addr = fmt.Sprintf("func:nextavailableip:%s,%s", ipv6cidr, netview)
	}
	recordHost := NewEmptyHostRecord()
	recordHostIpv6AddrSlice := []HostRecordIpv6Addr{}
	recordHostIpv4AddrSlice := []HostRecordIpv4Addr{}
	if ipv6Addr != "" {
		recordHostIpv6Addr := NewHostRecordIpv6Addr(ipv6Addr, duid, enabledhcp, "")
		recordHostIpv6AddrSlice = []HostRecordIpv6Addr{*recordHostIpv6Addr}
	}
	if ipv4Addr != "" {
		recordHostIpAddr := NewHostRecordIpv4Addr(ipv4Addr, macAddr, enabledhcp, "")
		recordHostIpv4AddrSlice = []HostRecordIpv4Addr{*recordHostIpAddr}
	}
	recordHost = NewHostRecord(
		netview, recordName, "", "", recordHostIpv4AddrSlice, recordHostIpv6AddrSlice,
		eas, enabledns, dnsview, "", "", useTtl, ttl, comment, aliases)
	ref, err := objMgr.connector.CreateObject(recordHost)
	if err != nil {
		return nil, err
	}
	recordHost.Ref = ref
	err = objMgr.connector.GetObject(
		recordHost, ref, NewQueryParams(false, nil), &recordHost)
	return recordHost, err
}

func (objMgr *ObjectManager) GetHostRecordByRef(ref string) (*HostRecord, error) {
	recordHost := NewEmptyHostRecord()
	err := objMgr.connector.GetObject(
		recordHost, ref, NewQueryParams(false, nil), &recordHost)
	return recordHost, err
}

func (objMgr *ObjectManager) GetHostRecord(netview string, dnsview string, recordName string, ipv4addr string, ipv6addr string) (*HostRecord, error) {
	var res []HostRecord

	recordHost := NewEmptyHostRecord()

	sf := map[string]string{
		"name": recordName,
	}
	if netview != "" {
		sf["network_view"] = netview
	}
	if dnsview != "" {
		sf["view"] = dnsview
	}
	if ipv4addr != "" {
		sf["ipv4addr"] = ipv4addr
	}
	if ipv6addr != "" {
		sf["ipv6addr"] = ipv6addr
	}
	queryParams := NewQueryParams(false, sf)
	err := objMgr.connector.GetObject(recordHost, "", queryParams, &res)

	if err != nil || res == nil || len(res) == 0 {
		return nil, err
	}
	return &res[0], err

}

func (objMgr *ObjectManager) GetIpAddressFromHostRecord(host HostRecord) (string, error) {
	err := objMgr.connector.GetObject(
		&host, host.Ref, NewQueryParams(false, nil), &host)
	return host.Ipv4Addrs[0].Ipv4Addr, err
}

func (objMgr *ObjectManager) UpdateHostRecord(
	hostRref string,
	enabledns bool,
	enabledhcp bool,
	name string,
	netView string,
	ipv4cidr string,
	ipv6cidr string,
	ipv4Addr string,
	ipv6Addr string,
	macAddress string,
	duid string,
	useTtl bool,
	ttl uint32,
	comment string,
	eas EA,
	aliases []string) (*HostRecord, error) {

	recordHostIpv4AddrSlice := []HostRecordIpv4Addr{}
	recordHostIpv6AddrSlice := []HostRecordIpv6Addr{}
	if ipv4Addr == "" {
		if ipv4cidr != "" {
			ip, _, err := net.ParseCIDR(ipv4cidr)
			if err != nil {
				return nil, fmt.Errorf("cannot parse CIDR value: %s", err.Error())
			}
			if ip.To4() == nil {
				return nil, fmt.Errorf("CIDR value must be an IPv4 CIDR, not an IPv6 one")
			}
			if netView == "" {
				netView = "default"
			}
			newIpAddr := fmt.Sprintf("func:nextavailableip:%s,%s", ipv4cidr, netView)
			recordHostIpAddr := NewHostRecordIpv4Addr(newIpAddr, macAddress, enabledhcp, "")
			recordHostIpv4AddrSlice = []HostRecordIpv4Addr{*recordHostIpAddr}
		}
	} else {
		ip := net.ParseIP(ipv4Addr)
		if ip == nil {
			return nil, fmt.Errorf("'IP address for the record is not valid")
		}
		if ip.To4() == nil {
			return nil, fmt.Errorf("IP address must be an IPv4 address, not an IPv6 one")
		}
		recordHostIpAddr := NewHostRecordIpv4Addr(ipv4Addr, macAddress, enabledhcp, "")
		recordHostIpv4AddrSlice = []HostRecordIpv4Addr{*recordHostIpAddr}
	}
	if ipv6Addr == "" {
		if ipv6cidr != "" {
			ip, _, err := net.ParseCIDR(ipv6cidr)
			if err != nil {
				return nil, fmt.Errorf("cannot parse CIDR value: %s", err.Error())
			}
			if ip.To4() != nil || ip.To16() == nil {
				return nil, fmt.Errorf("CIDR value must be an IPv6 CIDR, not an IPv4 one")
			}
			if netView == "" {
				netView = "default"
			}
			newIpAddr := fmt.Sprintf("func:nextavailableip:%s,%s", ipv6cidr, netView)
			recordHostIpAddr := NewHostRecordIpv6Addr(newIpAddr, duid, enabledhcp, "")
			recordHostIpv6AddrSlice = []HostRecordIpv6Addr{*recordHostIpAddr}
		}
	} else {
		ip := net.ParseIP(ipv6Addr)
		if ip == nil {
			return nil, fmt.Errorf("IP address for the record is not valid")
		}
		if ip.To4() != nil || ip.To16() == nil {
			return nil, fmt.Errorf("IP address must be an IPv6 address, not an IPv4 one")
		}
		recordHostIpAddr := NewHostRecordIpv6Addr(ipv6Addr, duid, enabledhcp, "")
		recordHostIpv6AddrSlice = []HostRecordIpv6Addr{*recordHostIpAddr}
	}
	updateHostRecord := NewHostRecord(
		"", name, "", "", recordHostIpv4AddrSlice, recordHostIpv6AddrSlice,
		eas, enabledns, "", "", hostRref, useTtl, ttl, comment, aliases)
	ref, err := objMgr.connector.UpdateObject(updateHostRecord, hostRref)

	updateHostRecord, err = objMgr.GetHostRecordByRef(ref)
	if err != nil {
		return nil, err
	}
	return updateHostRecord, nil
}

func (objMgr *ObjectManager) DeleteHostRecord(ref string) (string, error) {
	return objMgr.connector.DeleteObject(ref)
}

// UpdateNetwork updates comment and EA parameters.
// EAs which exist will be updated,
// those which do exist but not in setEas map, will be deleted,
// EAs which do not exist will be created as new.
func (objMgr *ObjectManager) UpdateNetwork(
	ref string,
	setEas EA,
	comment string) (*Network, error) {

	r := regexp.MustCompile("^ipv6network\\/.+")
	isIPv6 := r.MatchString(ref)

	nw := NewNetwork("", "", isIPv6, "", nil)
	err := objMgr.connector.GetObject(
		nw, ref, NewQueryParams(false, nil), nw)

	if err != nil {
		return nil, err
	}

	nw.Ea = setEas
	nw.Comment = comment

	newRef, err := objMgr.connector.UpdateObject(nw, ref)
	if err != nil {
		return nil, err
	}

	nw.Ref = newRef
	return nw, nil
}

func (objMgr *ObjectManager) UpdateNetworkContainer(
	ref string,
	setEas EA,
	comment string) (*NetworkContainer, error) {

	nc := &NetworkContainer{}
	nc.returnFields = []string{"extattrs", "comment"}

	err := objMgr.connector.GetObject(
		nc, ref, NewQueryParams(false, nil), nc)
	if err != nil {
		return nil, err
	}

	nc.Ea = setEas
	nc.Comment = comment

	reference, err := objMgr.connector.UpdateObject(nc, ref)
	if err != nil {
		return nil, err
	}

	nc.Ref = reference
	return nc, nil
}

func (objMgr *ObjectManager) CreateARecord(
	netView string,
	dnsView string,
	name string,
	cidr string,
	ipAddr string,
	ttl uint32,
	useTTL bool,
	comment string,
	eas EA) (*RecordA, error) {

	cleanName := strings.TrimSpace(name)
	if cleanName == "" || cleanName != name {
		return nil, fmt.Errorf(
			"'name' argument is expected to be non-empty and it must NOT contain leading/trailing spaces")
	}

	recordA := NewRecordA(dnsView, "", name, "", ttl, useTTL, comment, eas, "")

	if ipAddr == "" {
		if cidr == "" {
			return nil, fmt.Errorf("CIDR must not be empty")
		}
		ip, _, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("cannot parse CIDR value: %s", err.Error())
		}
		if ip.To4() == nil {
			return nil, fmt.Errorf("CIDR value must be an IPv4 CIDR, not an IPv6 one")
		}
		if netView == "" {
			recordA.Ipv4Addr = fmt.Sprintf("func:nextavailableip:%s", cidr)
		} else {
			recordA.Ipv4Addr = fmt.Sprintf("func:nextavailableip:%s,%s", cidr, netView)
		}
	} else {
		ip := net.ParseIP(ipAddr)
		if ip == nil {
			return nil, fmt.Errorf("'IP address for the record is not valid")
		}
		if ip.To4() == nil {
			return nil, fmt.Errorf("IP address must be an IPv4 address, not an IPv6 one")
		}
		recordA.Ipv4Addr = ipAddr
	}

	ref, err := objMgr.connector.CreateObject(recordA)
	if err != nil {
		return nil, err
	}

	newRec, err := objMgr.GetARecordByRef(ref)
	if err != nil {
		return nil, err
	}

	return newRec, nil
}

func (objMgr *ObjectManager) UpdateARecord(
	ref string,
	name string,
	ipAddr string,
	cidr string,
	netView string,
	ttl uint32,
	useTTL bool,
	comment string,
	eas EA) (*RecordA, error) {

	cleanName := strings.ToLower(strings.TrimSpace(name))
	if cleanName == "" || cleanName != name {
		return nil, fmt.Errorf(
			"'name' argument is expected to be non-empty and it must NOT contain leading/trailing spaces")
	}

	rec, err := objMgr.GetARecordByRef(ref)
	if err != nil {
		return nil, err
	}
	newIpAddr := rec.Ipv4Addr
	if ipAddr == "" {
		if cidr != "" {
			ip, _, err := net.ParseCIDR(cidr)
			if err != nil {
				return nil, fmt.Errorf("cannot parse CIDR value: %s", err.Error())
			}
			if ip.To4() == nil {
				return nil, fmt.Errorf("CIDR value must be an IPv4 CIDR, not an IPv6 one")
			}
			if netView == "" {
				newIpAddr = fmt.Sprintf("func:nextavailableip:%s", cidr)
			} else {
				newIpAddr = fmt.Sprintf("func:nextavailableip:%s,%s", cidr, netView)
			}
		}
		// else: leaving ipv4addr field untouched
	} else {
		ip := net.ParseIP(ipAddr)
		if ip == nil {
			return nil, fmt.Errorf("'IP address for the record is not valid")
		}
		if ip.To4() == nil {
			return nil, fmt.Errorf("IP address must be an IPv4 address, not an IPv6 one")
		}
		newIpAddr = ipAddr
	}
	rec = NewRecordA(
		"", "", name, newIpAddr, ttl, useTTL, comment, eas, ref)
	ref, err = objMgr.connector.UpdateObject(rec, ref)
	if err != nil {
		return nil, err
	}
	rec.Ref = ref

	rec, err = objMgr.GetARecordByRef(ref)
	if err != nil {
		return nil, err
	}

	return rec, nil
}

func (objMgr *ObjectManager) GetARecord(dnsview string, recordName string, ipAddr string) (*RecordA, error) {
	var res []RecordA
	recordA := NewEmptyRecordA()
	if dnsview == "" || recordName == "" || ipAddr == "" {
		return nil, fmt.Errorf("DNS view, IPv4 address and record name of the record are required to retreive a unique A record")
	}
	sf := map[string]string{
		"view":     dnsview,
		"name":     recordName,
		"ipv4addr": ipAddr,
	}
	queryParams := NewQueryParams(false, sf)
	err := objMgr.connector.GetObject(recordA, "", queryParams, &res)

	if err != nil {
		return nil, err
	} else if res == nil || len(res) == 0 {
		return nil, NewNotFoundError(
			fmt.Sprintf(
				"A record with name '%s' and IPv4 address '%s' in DNS view '%s' is not found",
				recordName, ipAddr, dnsview))
	}
	return &res[0], nil
}

func (objMgr *ObjectManager) GetARecordByRef(ref string) (*RecordA, error) {
	recordA := NewEmptyRecordA()
	err := objMgr.connector.GetObject(
		recordA, ref, NewQueryParams(false, nil), &recordA)
	return recordA, err
}

func (objMgr *ObjectManager) DeleteARecord(ref string) (string, error) {
	return objMgr.connector.DeleteObject(ref)
}

func (objMgr *ObjectManager) CreateAAAARecord(
	netView string,
	dnsView string,
	recordName string,
	cidr string,
	ipAddr string,
	useTtl bool,
	ttl uint32,
	comment string,
	eas EA) (*RecordAAAA, error) {

	cleanName := strings.ToLower(strings.TrimSpace(recordName))
	if cleanName == "" || cleanName != recordName {
		return nil, fmt.Errorf(
			"'name' argument is expected to be non-empty and it must NOT contain leading/trailing spaces")
	}
	recordAAAA := NewRecordAAAA(dnsView, recordName, "", useTtl, ttl, comment, eas, "")

	if ipAddr == "" {
		if cidr == "" {
			return nil, fmt.Errorf("CIDR must not be empty")
		}
		ipAddress, _, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, fmt.Errorf("cannot parse CIDR value: %s", err.Error())
		}
		if ipAddress.To4() != nil || ipAddress.To16() == nil {
			return nil, fmt.Errorf("CIDR value must be an IPv6 CIDR, not an IPv4 one")
		}
		if netView == "" {
			netView = "default"
		}
		recordAAAA.Ipv6Addr = fmt.Sprintf("func:nextavailableip:%s,%s", cidr, netView)
	} else {
		ipAddress := net.ParseIP(ipAddr)
		if ipAddress == nil {
			return nil, fmt.Errorf("IP address for the record is not valid")
		}
		if ipAddress.To4() != nil || ipAddress.To16() == nil {
			return nil, fmt.Errorf("IP address must be an IPv6 address, not an IPv4 one")
		}
		recordAAAA.Ipv6Addr = ipAddr
	}
	ref, err := objMgr.connector.CreateObject(recordAAAA)
	if err != nil {
		return nil, err
	}
	recordAAAA, err = objMgr.GetAAAARecordByRef(ref)
	if err != nil {
		return nil, err
	}
	return recordAAAA, nil
}

func (objMgr *ObjectManager) GetAAAARecord(dnsview string, recordName string, ipAddr string) (*RecordAAAA, error) {
	var res []RecordAAAA
	recordAAAA := NewEmptyRecordAAAA()
	if dnsview == "" || recordName == "" || ipAddr == "" {
		return nil, fmt.Errorf("DNS view, IPv6 address and record name of the record are required to retreive a unique AAAA record")
	}
	sf := map[string]string{
		"view":     dnsview,
		"name":     recordName,
		"ipv6addr": ipAddr,
	}
	queryParams := NewQueryParams(false, sf)
	err := objMgr.connector.GetObject(recordAAAA, "", queryParams, &res)

	if err != nil {
		return nil, err
	} else if res == nil || len(res) == 0 {
		return nil, NewNotFoundError(
			fmt.Sprintf(
				"AAAA record with name '%s' and IPv6 address '%s' in DNS view '%s' is not found",
				recordName, ipAddr, dnsview))
	}
	return &res[0], nil
}

func (objMgr *ObjectManager) GetAAAARecordByRef(ref string) (*RecordAAAA, error) {
	recordAAAA := NewEmptyRecordAAAA()
	err := objMgr.connector.GetObject(
		recordAAAA, ref, NewQueryParams(false, nil), &recordAAAA)
	return recordAAAA, err
}

func (objMgr *ObjectManager) DeleteAAAARecord(ref string) (string, error) {
	return objMgr.connector.DeleteObject(ref)
}

func (objMgr *ObjectManager) UpdateAAAARecord(
	ref string,
	netView string,
	recordName string,
	ipAddr string,
	cidr string,
	useTtl bool,
	ttl uint32,
	comment string,
	setEas EA) (*RecordAAAA, error) {

	cleanName := strings.ToLower(strings.TrimSpace(recordName))
	if cleanName == "" || cleanName != recordName {
		return nil, fmt.Errorf(
			"'name' argument is expected to be non-empty and it must NOT contain leading/trailing spaces")
	}

	rec, err := objMgr.GetAAAARecordByRef(ref)
	if err != nil {
		return nil, err
	}
	newIpAddr := rec.Ipv6Addr
	if ipAddr == "" {
		if cidr != "" {
			ipAddress, _, err := net.ParseCIDR(cidr)
			if err != nil {
				return nil, fmt.Errorf("cannot parse CIDR value: %s", err.Error())
			}
			if ipAddress.To4() != nil || ipAddress.To16() == nil {
				return nil, fmt.Errorf("CIDR value must be an IPv6 CIDR, not an IPv4 one")
			}
			if netView == "" {
				netView = "default"
			}
			newIpAddr = fmt.Sprintf("func:nextavailableip:%s,%s", cidr, netView)
		}
	} else {
		ipAddress := net.ParseIP(ipAddr)
		if ipAddress == nil {
			return nil, fmt.Errorf("IP address for the record is not valid")
		}
		if ipAddress.To4() != nil || ipAddress.To16() == nil {
			return nil, fmt.Errorf("IP address must be an IPv6 address, not an IPv4 one")
		}
		newIpAddr = ipAddr
	}
	recordAAAA := NewRecordAAAA("", recordName, newIpAddr, useTtl, ttl, comment, setEas, ref)
	reference, err := objMgr.connector.UpdateObject(recordAAAA, ref)
	if err != nil {
		return nil, err
	}

	recordAAAA, err = objMgr.GetAAAARecordByRef(reference)
	if err != nil {
		return nil, err
	}
	return recordAAAA, nil
}

func (objMgr *ObjectManager) CreateCNAMERecord(
	dnsview string,
	canonical string,
	recordname string,
	useTtl bool,
	ttl uint32,
	comment string,
	eas EA) (*RecordCNAME, error) {

	if canonical == "" || recordname == "" {
		return nil, fmt.Errorf("canonical name and record name fields are required to create a CNAME record")
	}
	recordCNAME := NewRecordCNAME(dnsview, canonical, recordname, useTtl, ttl, comment, eas, "")

	ref, err := objMgr.connector.CreateObject(recordCNAME)
	recordCNAME, err = objMgr.GetCNAMERecordByRef(ref)
	return recordCNAME, err
}

func (objMgr *ObjectManager) GetCNAMERecord(dnsview string, canonical string, recordName string) (*RecordCNAME, error) {
	var res []RecordCNAME
	recordCNAME := NewEmptyRecordCNAME()
	if dnsview == "" || canonical == "" || recordName == "" {
		return nil, fmt.Errorf("DNS view, canonical name and record name of the record are required to retreive a unique CNAME record")
	}
	sf := map[string]string{
		"view":      dnsview,
		"canonical": canonical,
		"name":      recordName,
	}

	queryParams := NewQueryParams(false, sf)
	err := objMgr.connector.GetObject(recordCNAME, "", queryParams, &res)

	if err != nil {
		return nil, err
	} else if res == nil || len(res) == 0 {
		return nil, NewNotFoundError(
			fmt.Sprintf(
				"CNAME record with name '%s' and canonical name '%s' in DNS view '%s' is not found",
				recordName, canonical, dnsview))
	}
	return &res[0], nil
}

func (objMgr *ObjectManager) GetCNAMERecordByRef(ref string) (*RecordCNAME, error) {
	recordCNAME := NewEmptyRecordCNAME()
	err := objMgr.connector.GetObject(
		recordCNAME, ref, NewQueryParams(false, nil), &recordCNAME)
	return recordCNAME, err
}

func (objMgr *ObjectManager) DeleteCNAMERecord(ref string) (string, error) {
	return objMgr.connector.DeleteObject(ref)
}

func (objMgr *ObjectManager) UpdateCNAMERecord(
	ref string,
	canonical string,
	recordName string,
	useTtl bool,
	ttl uint32,
	comment string,
	setEas EA) (*RecordCNAME, error) {

	recordCNAME := NewRecordCNAME("", canonical, recordName, useTtl, ttl, comment, setEas, ref)
	updatedRef, err := objMgr.connector.UpdateObject(recordCNAME, ref)
	if err != nil {
		return nil, err
	}

	recordCNAME, err = objMgr.GetCNAMERecordByRef(updatedRef)
	return recordCNAME, err
}

// Creates TXT Record. Use TTL of 0 to inherit TTL from the Zone
func (objMgr *ObjectManager) CreateTXTRecord(recordname string, text string, ttl uint, dnsview string) (*RecordTXT, error) {

	recordTXT := NewRecordTXT(RecordTXT{
		View: dnsview,
		Name: recordname,
		Text: text,
		Ttl:  ttl,
	})

	ref, err := objMgr.connector.CreateObject(recordTXT)
	recordTXT.Ref = ref
	return recordTXT, err
}

func (objMgr *ObjectManager) GetTXTRecordByRef(ref string) (*RecordTXT, error) {
	recordTXT := NewRecordTXT(RecordTXT{})
	err := objMgr.connector.GetObject(
		recordTXT, ref, NewQueryParams(false, nil), &recordTXT)
	return recordTXT, err
}

func (objMgr *ObjectManager) GetTXTRecord(name string) (*RecordTXT, error) {
	if name == "" {
		return nil, fmt.Errorf("name can not be empty")
	}
	var res []RecordTXT

	recordTXT := NewRecordTXT(RecordTXT{})

	sf := map[string]string{
		"name": name,
	}
	queryParams := NewQueryParams(false, sf)
	err := objMgr.connector.GetObject(recordTXT, "", queryParams, &res)

	if err != nil || res == nil || len(res) == 0 {
		return nil, err
	}

	return &res[0], nil
}

func (objMgr *ObjectManager) UpdateTXTRecord(recordname string, text string) (*RecordTXT, error) {
	var res []RecordTXT

	recordTXT := NewRecordTXT(RecordTXT{Name: recordname})

	sf := map[string]string{
		"name": recordname,
	}
	queryParams := NewQueryParams(false, sf)
	err := objMgr.connector.GetObject(recordTXT, "", queryParams, &res)

	if len(res) == 0 {
		return nil, nil
	}

	res[0].Text = text

	res[0].Zone = "" //  set the Zone value to "" as its a non writable field

	_, err = objMgr.connector.UpdateObject(&res[0], res[0].Ref)

	if err != nil || res == nil || len(res) == 0 {
		return nil, err
	}

	return &res[0], nil
}

func (objMgr *ObjectManager) DeleteTXTRecord(ref string) (string, error) {
	return objMgr.connector.DeleteObject(ref)
}

// TODO check if the respective zone exists before creation of the record
func (objMgr *ObjectManager) CreatePTRRecord(
	networkView string,
	dnsView string,
	ptrdname string,
	recordName string,
	cidr string,
	ipAddr string,
	useTtl bool,
	ttl uint32,
	comment string,
	eas EA) (*RecordPTR, error) {

	if ptrdname == "" {
		return nil, fmt.Errorf("ptrdname is a required field to create a PTR record")
	}
	recordPTR := NewRecordPTR(dnsView, ptrdname, useTtl, ttl, comment, eas)

	if ipAddr == "" && cidr != "" {
		if networkView == "" {
			networkView = "default"
		}
		ipAddress, net, err := net.ParseCIDR(cidr)
		if err != nil {
			return nil, err
		}
		if ipAddress.To4() != nil {
			if net.String() != cidr {
				return nil, fmt.Errorf("%s is an invalid CIDR. Note: leading zeros should be removed if exists", cidr)
			}
			recordPTR.Ipv4Addr = fmt.Sprintf("func:nextavailableip:%s,%s", cidr, networkView)
		} else {
			recordPTR.Ipv6Addr = fmt.Sprintf("func:nextavailableip:%s,%s", cidr, networkView)
		}
	} else if ipAddr != "" {
		ipAddress := net.ParseIP(ipAddr)
		if ipAddress == nil {
			return nil, fmt.Errorf("%s is an invalid IP address", ipAddr)
		}
		if ipAddress.To4() != nil {
			recordPTR.Ipv4Addr = ipAddr
		} else {
			recordPTR.Ipv6Addr = ipAddr
		}
	} else if recordName != "" {
		recordPTR.Name = recordName
	} else {
		return nil, fmt.Errorf("CIDR and network view are required to allocate a next available IP address\n" +
			"IP address is required to create PTR record in reverse mapping zone\n" +
			"record name is required to create a record in forwarrd mapping zone")
	}
	ref, err := objMgr.connector.CreateObject(recordPTR)
	if err != nil {
		return nil, err
	}
	recordPTR, err = objMgr.GetPTRRecordByRef(ref)
	return recordPTR, err
}

func (objMgr *ObjectManager) GetPTRRecord(dnsview string, ptrdname string, recordName string, ipAddr string) (*RecordPTR, error) {
	var res []RecordPTR
	recordPtr := NewEmptyRecordPTR()
	sf := map[string]string{
		"view":     dnsview,
		"ptrdname": ptrdname,
	}
	cleanName := strings.TrimSpace(recordName)
	if ipAddr != "" {
		ipAddress := net.ParseIP(ipAddr)
		if ipAddress == nil {
			return nil, fmt.Errorf("%s is an invalid IP address", ipAddr)
		}
		if ipAddress.To4() != nil {
			sf["ipv4addr"] = ipAddr
		} else {
			sf["ipv6addr"] = ipAddr
		}
	} else if cleanName != "" {
		sf["name"] = cleanName
	} else {
		return nil, fmt.Errorf("record name or IP Address of the record has to be passed to get a unique record")
	}
	queryParams := NewQueryParams(false, sf)
	err := objMgr.connector.GetObject(recordPtr, "", queryParams, &res)

	if err != nil {
		return nil, err
	} else if res == nil || len(res) == 0 {
		return nil, NewNotFoundError(
			fmt.Sprintf(
				"PTR record with name/IP '%v' and ptrdname '%s' in DNS view '%s' is not found",
				[]string{recordName, ipAddr}, ptrdname, dnsview))
	}
	return &res[0], nil
}

func (objMgr *ObjectManager) GetPTRRecordByRef(ref string) (*RecordPTR, error) {
	recordPTR := NewEmptyRecordPTR()
	err := objMgr.connector.GetObject(
		recordPTR, ref, NewQueryParams(false, nil), &recordPTR)
	return recordPTR, err
}

func (objMgr *ObjectManager) DeletePTRRecord(ref string) (string, error) {
	return objMgr.connector.DeleteObject(ref)
}

func (objMgr *ObjectManager) UpdatePTRRecord(
	ref string,
	netview string,
	ptrdname string,
	name string,
	cidr string,
	ipAddr string,
	useTtl bool,
	ttl uint32,
	comment string,
	setEas EA) (*RecordPTR, error) {

	recordPTR := NewRecordPTR("", ptrdname, useTtl, ttl, comment, setEas)
	recordPTR.Ref = ref
	recordPTR.Name = name
	isIPv6, _ := regexp.MatchString(`^record:ptr/.+.ip6.arpa/.+`, ref)

	if ipAddr == "" {
		if cidr != "" {
			ipAddress, _, err := net.ParseCIDR(cidr)
			if err != nil {
				return nil, fmt.Errorf("cannot parse CIDR value: %s", err.Error())
			}
			if netview == "" {
				netview = "default"
			}
			if isIPv6 {
				if ipAddress.To4() != nil || ipAddress.To16() == nil {
					return nil, fmt.Errorf("CIDR value must be an IPv6 CIDR, not an IPv4 one")
				}
				recordPTR.Ipv6Addr = fmt.Sprintf("func:nextavailableip:%s,%s", cidr, netview)
			} else {
				if ipAddress.To4() == nil {
					return nil, fmt.Errorf("CIDR value must be an IPv4 CIDR, not an IPv6 one")
				}
				recordPTR.Ipv4Addr = fmt.Sprintf("func:nextavailableip:%s,%s", cidr, netview)
			}
		}
	} else {
		ipAddress := net.ParseIP(ipAddr)
		if ipAddress == nil {
			return nil, fmt.Errorf("IP address for the record is not valid")
		}
		if isIPv6 {
			if ipAddress.To4() != nil || ipAddress.To16() == nil {
				return nil, fmt.Errorf("IP address must be an IPv6 address, not an IPv4 one")
			}
			recordPTR.Ipv6Addr = ipAddr
		} else {
			if ipAddress.To4() == nil {
				return nil, fmt.Errorf("IP address must be an IPv4 address, not an IPv6 one")
			}
			recordPTR.Ipv4Addr = ipAddr
		}
	}
	reference, err := objMgr.connector.UpdateObject(recordPTR, ref)
	if err != nil {
		return nil, err
	}

	recordPTR, err = objMgr.GetPTRRecordByRef(reference)
	return recordPTR, err
}

// CreateMultiObject unmarshals the result into slice of maps
func (objMgr *ObjectManager) CreateMultiObject(req *MultiRequest) ([]map[string]interface{}, error) {

	conn := objMgr.connector.(*Connector)
	queryParams := NewQueryParams(false, nil)
	res, err := conn.makeRequest(CREATE, req, "", queryParams)

	if err != nil {
		return nil, err
	}

	var result []map[string]interface{}
	err = json.Unmarshal(res, &result)

	if err != nil {
		return nil, err
	}

	return result, nil
}

// GetUpgradeStatus returns the grid upgrade information
func (objMgr *ObjectManager) GetUpgradeStatus(statusType string) ([]UpgradeStatus, error) {
	var res []UpgradeStatus

	if statusType == "" {
		// TODO option may vary according to the WAPI version, need to
		// throw relevant  error.
		msg := fmt.Sprintf("Status type can not be nil")
		return res, errors.New(msg)
	}
	upgradestatus := NewUpgradeStatus(UpgradeStatus{})

	sf := map[string]string{
		"type": statusType,
	}
	queryParams := NewQueryParams(false, sf)
	err := objMgr.connector.GetObject(upgradestatus, "", queryParams, &res)

	return res, err
}

// GetAllMembers returns all members information
func (objMgr *ObjectManager) GetAllMembers() ([]Member, error) {
	var res []Member

	memberObj := NewMember(Member{})
	err := objMgr.connector.GetObject(
		memberObj, "", NewQueryParams(false, nil), &res)
	return res, err
}

// GetCapacityReport returns all capacity for members
func (objMgr *ObjectManager) GetCapacityReport(name string) ([]CapacityReport, error) {
	var res []CapacityReport

	capacityReport := NewCapcityReport(CapacityReport{})

	sf := map[string]string{
		"name": name,
	}
	queryParams := NewQueryParams(false, sf)
	err := objMgr.connector.GetObject(capacityReport, "", queryParams, &res)
	return res, err
}

// GetLicense returns the license details for member
func (objMgr *ObjectManager) GetLicense() ([]License, error) {
	var res []License

	licenseObj := NewLicense(License{})
	err := objMgr.connector.GetObject(
		licenseObj, "", NewQueryParams(false, nil), &res)
	return res, err
}

// GetLicense returns the license details for grid
func (objMgr *ObjectManager) GetGridLicense() ([]License, error) {
	var res []License

	licenseObj := NewGridLicense(License{})
	err := objMgr.connector.GetObject(
		licenseObj, "", NewQueryParams(false, nil), &res)
	return res, err
}

// GetGridInfo returns the details for grid
func (objMgr *ObjectManager) GetGridInfo() ([]Grid, error) {
	var res []Grid

	gridObj := NewGrid(Grid{})
	err := objMgr.connector.GetObject(
		gridObj, "", NewQueryParams(false, nil), &res)
	return res, err
}

// CreateZoneAuth creates zones and subs by passing fqdn
func (objMgr *ObjectManager) CreateZoneAuth(
	fqdn string,
	eas EA) (*ZoneAuth, error) {

	zoneAuth := NewZoneAuth(ZoneAuth{
		Fqdn: fqdn,
		Ea:   eas})

	ref, err := objMgr.connector.CreateObject(zoneAuth)
	zoneAuth.Ref = ref
	return zoneAuth, err
}

// Retreive a authortative zone by ref
func (objMgr *ObjectManager) GetZoneAuthByRef(ref string) (*ZoneAuth, error) {
	res := NewZoneAuth(ZoneAuth{})

	if ref == "" {
		return nil, fmt.Errorf("empty reference to an object is not allowed")
	}

	err := objMgr.connector.GetObject(
		res, ref, NewQueryParams(false, nil), res)
	return res, err
}

// DeleteZoneAuth deletes an auth zone
func (objMgr *ObjectManager) DeleteZoneAuth(ref string) (string, error) {
	return objMgr.connector.DeleteObject(ref)
}

// GetZoneAuth returns the authoritatives zones
func (objMgr *ObjectManager) GetZoneAuth() ([]ZoneAuth, error) {
	var res []ZoneAuth

	zoneAuth := NewZoneAuth(ZoneAuth{})
	err := objMgr.connector.GetObject(
		zoneAuth, "", NewQueryParams(false, nil), &res)

	return res, err
}

// GetZoneDelegated returns the delegated zone
func (objMgr *ObjectManager) GetZoneDelegated(fqdn string) (*ZoneDelegated, error) {
	if len(fqdn) == 0 {
		return nil, nil
	}
	var res []ZoneDelegated

	zoneDelegated := NewZoneDelegated(ZoneDelegated{})

	sf := map[string]string{
		"fqdn": fqdn,
	}
	queryParams := NewQueryParams(false, sf)
	err := objMgr.connector.GetObject(zoneDelegated, "", queryParams, &res)

	if err != nil || res == nil || len(res) == 0 {
		return nil, err
	}

	return &res[0], nil
}

// CreateZoneDelegated creates delegated zone
func (objMgr *ObjectManager) CreateZoneDelegated(fqdn string, delegate_to []NameServer) (*ZoneDelegated, error) {
	zoneDelegated := NewZoneDelegated(ZoneDelegated{
		Fqdn:       fqdn,
		DelegateTo: delegate_to})

	ref, err := objMgr.connector.CreateObject(zoneDelegated)
	zoneDelegated.Ref = ref

	return zoneDelegated, err
}

// UpdateZoneDelegated updates delegated zone
func (objMgr *ObjectManager) UpdateZoneDelegated(ref string, delegate_to []NameServer) (*ZoneDelegated, error) {
	zoneDelegated := NewZoneDelegated(ZoneDelegated{
		Ref:        ref,
		DelegateTo: delegate_to})

	refResp, err := objMgr.connector.UpdateObject(zoneDelegated, ref)
	zoneDelegated.Ref = refResp
	return zoneDelegated, err
}

// DeleteZoneDelegated deletes delegated zone
func (objMgr *ObjectManager) DeleteZoneDelegated(ref string) (string, error) {
	return objMgr.connector.DeleteObject(ref)
}
