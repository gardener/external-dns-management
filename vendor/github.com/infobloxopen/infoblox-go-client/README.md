# Infoblox Go Client

An Infoblox Client library for Go.

This library is compatible with Go 1.2+

- [Prerequisites](#Prerequisites)
- [Installation](#Installation)
- [Usage](#Usage)

## Prerequisites
   * Infoblox GRID with 2.5 or above WAPI support
   * Go 1.2 or above

## Installation
   go get github.com/infobloxopen/infoblox-go-client

## Usage

   The following is a very simple example for the client usage:

       package main
       import (
   	    "fmt"
   	    ibclient "github.com/infobloxopen/infoblox-go-client"
       )

       func main() {
   	    hostConfig := ibclient.HostConfig{
   		    Host:     "<NIOS grid IP>",
   		    Version:  "<WAPI version>",
   		    Port:     "PORT",
   		    Username: "username",
   		    Password: "password",
   	    }
   	    transportConfig := ibclient.NewTransportConfig("false", 20, 10)
   	    requestBuilder := &ibclient.WapiRequestBuilder{}
   	    requestor := &ibclient.WapiHttpRequestor{}
   	    conn, err := ibclient.NewConnector(hostConfig, transportConfig, requestBuilder, requestor)
   	    if err != nil {
   		    fmt.Println(err)
   	    }
   	    defer conn.Logout()
   	    objMgr := ibclient.NewObjectManager(conn, "myclient", "")
   	    //Fetches grid information
   	    fmt.Println(objMgr.GetLicense())
       }
       
## Example of CRUD Operations on A Record:
CREATE
	
	fmt.Println(objMgr.CreateARecord(ibclient.RecordA{Name:"myRecord.myZone.com", View: "myDNSView", Ipv4Addr: "192.168.2.7"}))
	
GET A Record by passing Reference, Name or IPv4Addr

	fmt.Println(objMgr.GetARecord(ibclient.RecordA{Name: "myRecord.myZone.com"}))
	fmt.Println(objMgr.GetARecord(ibclient.RecordA{IPv4Addr: "192.168.2.7"}))
	fmt.Println(objMgr.GetARecord(ibclient.RecordA{Ref: "record:a/ZG5zLmJpbmRfYSQuMTguY29tLnRlc3QsaW5mbzEsMTkyLjE2OS4yLjU:myRecord.myZone.com/myDNSView"}))
	
UPDATE IP Address or Name or Extensible Attributes
	
	fmt.Println(objMgr.UpdateARecord(ibclient.RecordA{Ref: "record:a/ZG5zLmJpbmRfYSQuMTguY29tLnRlc3QsaW5mbzEsMTkyLjE2OS4yLjU:myRecord.myZone.com/myDNSView", Ipv4Addr: "192.168.2.3"})
	fmt.Println(objMgr.UpdateARecord(ibclient.RecordA{Ref: "record:a/ZG5zLmJpbmRfYSQuMTguY29tLnRlc3QsaW5mbzEsMTkyLjE2OS4yLjU:myRecord.myZone.com/myDNSView", Name: "updatedName.myZone.com"})
	ea := ibclient.EA{"Cloud API Owned": ibclient.Bool(false)}
	fmt.Println(objMgr.UpdateARecord(ibclient.RecordA{Ref: "record:a/ZG5zLmJpbmRfYSQuMTguY29tLnRlc3QsaW5mbzEsMTkyLjE2OS4yLjU:myRecord.myZone.com/myDNSView", Ea: ea})
DELETE A record by passing Reference or Name or Ipv4Addr
 
 	fmt.Println(objMgr.DeleteARecord(ibclient.RecordA{Name: "myRecord.myZone.com"}))
	fmt.Println(objMgr.DeleteARecord(ibclient.RecordA{IPv4Addr: "192.168.2.7"}))
	fmt.Println(objMgr.DeleteARecord(ibclient.RecordA{Ref: "record:a/ZG5zLmJpbmRfYSQuMTguY29tLnRlc3QsaW5mbzEsMTkyLjE2OS4yLjU:myRecord.myZone.com/myDNSView"}))
  
## Supported NIOS operations

   * CreateNetworkView
   * CreateDefaultNetviews
   * CreateNetwork
   * CreateNetworkContainer
   * GetNetworkView
   * GetNetwork
   * GetNetworkContainer
   * AllocateNetwork
   * UpdateFixedAddress
   * GetFixedAddress
   * ReleaseIP
   * DeleteNetwork
   * GetEADefinition
   * CreateEADefinition
   * UpdateNetworkViewEA
   * GetCapacityReport
   * GetAllMembers
   * GetUpgradeStatus (2.7 or above)
### New Methods
   * GET by Name or Ipv4Address or view
   * DELETE by Name or Ipv4Address
   * UPDATE Name, Ipv4Address or Extensible Attributes of A Record
