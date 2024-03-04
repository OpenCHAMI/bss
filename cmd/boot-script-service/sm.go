// MIT License
//
// (C) Copyright [2021] Hewlett Packard Enterprise Development LP
//
// Permission is hereby granted, free of charge, to any person obtaining a
// copy of this software and associated documentation files (the "Software"),
// to deal in the Software without restriction, including without limitation
// the rights to use, copy, modify, merge, publish, distribute, sublicense,
// and/or sell copies of the Software, and to permit persons to whom the
// Software is furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included
// in all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL
// THE AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR
// OTHER LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE,
// ARISING FROM, OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR
// OTHER DEALINGS IN THE SOFTWARE.

//
// Shasta State Manager interface code.
//
// Retrieve node info from the Hardware State Manager (HSM)
// Support retrievel from SQLite3 database as an alternative.
//

package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	base "github.com/Cray-HPE/hms-base"
	"github.com/OpenCHAMI/smd/v2/pkg/rf"
	"github.com/OpenCHAMI/smd/v2/pkg/sm"
)

const (
	badMAC       = "not available"
	undefinedMAC = "ff:ff:ff:ff:ff:ff"
	hsmTestEP    = "/Inventory/RedfishEndpoints"
)

type SMComponent struct {
	base.Component
	Fqdn            string   `json:"FQDN"`
	Mac             []string `json:"MAC"`
	EndpointEnabled bool     `json:"EndpointEnabled"`
}

type SMData struct {
	Components []SMComponent                    `json:"Components"`
	IPAddrs    map[string]sm.CompEthInterfaceV2 `json:"IPAddresses"`
}

var (
	smMutex     sync.Mutex
	smData      *SMData
	smClient    *OAuthClient
	smDataMap   map[string]SMComponent
	smBaseURL   string
	smJSONFile  string
	smTimeStamp int64
)

func makeSmMap(state *SMData) map[string]SMComponent {
	m := make(map[string]SMComponent)
	for _, v := range state.Components {
		m[v.ID] = v
	}
	return m
}

func TestSMAuthEnabled(retryCount, retryInterval uint64) (authEnabled bool, err error) {
	var (
		testURL string
		resp    *http.Response
	)

	if smClient == nil {
		err = fmt.Errorf("smClient nil. Has a connection been opened yet?")
		return
	}

	// If this endpoint is protected (querying it returns a 401),
	// auth is enabled.
	testURL, err = url.JoinPath(smBaseURL, hsmTestEP)
	if err != nil {
		err = fmt.Errorf("Could not join URL paths %q and %q: %v", smBaseURL, hsmTestEP, err)
		return
	}

	retryDuration, err := time.ParseDuration(fmt.Sprintf("%ds", retryInterval))
	if err != nil {
		err = fmt.Errorf("Invalid retry interval: %v", err)
		return
	}
	for retry := uint64(0); retry < retryCount; retry++ {
		log.Printf("Attempting connection to %s (attempt %d/%d)", testURL, retry+1, retryCount)
		resp, err = smClient.Get(testURL)
		if err != nil {
			err = fmt.Errorf("Could not GET %q: %v", testURL, err)
			time.Sleep(retryDuration)
			continue
		}
		log.Printf("Connected to %s on attempt %d", testURL, retry+1)
		if resp.StatusCode == 401 {
			authEnabled = true
		} else {
			authEnabled = false
		}

		return
	}

	err = fmt.Errorf("Number of retries (%d) exhausted when testing if SMD auth is enabled", retryCount)
	return
}

func TestSMProtectedAccess() error {
	var (
		req *http.Request
		res *http.Response
	)

	if accessToken == "" {
		return fmt.Errorf("Access token is empty")
	}
	if smClient == nil {
		return fmt.Errorf("smClient nil. Has a connection been opened yet?")
	}

	testURL, err := url.JoinPath(smBaseURL, hsmTestEP)
	if err != nil {
		err = fmt.Errorf("Could not join URL paths %q and %q: %v", smBaseURL, hsmTestEP, err)
		return err
	}

	req, err = http.NewRequest(http.MethodGet, testURL, nil)
	headers := map[string][]string{
		"Authorization": {"Bearer " + accessToken},
	}
	req.Header = headers
	res, err = smClient.Do(req)
	if err != nil {
		return fmt.Errorf("Could not execute request: %v")
	}
	defer res.Body.Close()

	return nil
}

func SmOpen(base, options string) error {
	u, err := url.Parse(base)
	if err != nil {
		return fmt.Errorf("Unknown HSM URL: %s", base)
	}
	if u.Scheme == "mem" {
		// The mem: interface to the state manager is strictly for testing
		// purposes.  A canned set of pre-defined nodes are loaded into memory
		// and used as state manager data.  This allows for testing of a larger
		// set of nodes than is currently readily available.
		debugf("Setting internal HSM data")
		buf := bytes.NewBufferString(state_manager_data_temp)
		dec := json.NewDecoder(buf)
		var comps SMData
		err = dec.Decode(&comps)
		if err != nil {
			debugf("Internal data conversion failure: %v", err)
		}
		smData = &comps
		smDataMap = makeSmMap(smData)
		return nil
	}
	if u.Scheme == "file" {
		// The file: interface allows for another method of testing with a
		// little more flexibilty than the mem: interface, but not quite as
		// stand-alone.
		smJSONFile = u.Path
		debugf("Setting externel HSM data file: %s", smJSONFile)
		return nil
	}
	https := u.Scheme == "https"

	// Right now, there is only one recognizable option, the
	// insecure option.  To allow for furture expansion, we
	// assume there will be a comma separated list of options.
	insecure := false
	for _, opt := range strings.Split(options, ",") {
		if strings.ToLower(opt) == "insecure" {
			insecure = true
			break
		}
	}
	// Using the Datastore service
	smClient = new(OAuthClient)
	if https && insecure {
		tcfg := new(tls.Config)
		tcfg.InsecureSkipVerify = true
		trans := new(http.Transport)
		trans.TLSClientConfig = tcfg
		smClient.Transport = trans
		log.Printf("WARNING: insecure https connection to state manager service\n")
	}
	smBaseURL = base + "/hsm/v2"
	log.Printf("Accessing state manager via %s\n", smBaseURL)

	var smAuthEnabled bool
	smAuthEnabled, err = TestSMAuthEnabled(authRetryCount, authRetryWait)
	if err != nil {
		return fmt.Errorf("Failed testing if HSM auth is enabled: %v", err)
	}
	if smAuthEnabled {
		log.Printf("HSM authenticated endpoints enabled, checking token")
		err = smClient.JWTTestAndRefresh()
		if err != nil {
			return fmt.Errorf("Failed refreshing JWT: %v", err)
		}
	}
	return nil
}

func getMacs(comp *SMComponent, eth []*rf.EthernetNICInfo) {
	for _, e := range eth {
		if e.MACAddress == "" || strings.EqualFold(e.MACAddress, badMAC) {
			continue
		}
		found := false
		for _, m := range comp.Mac {
			if m == e.MACAddress {
				found = true
				break
			}
		}
		if !found {
			comp.Mac = append(comp.Mac, e.MACAddress)
		}
	}
}

func ensureLegalMAC(mac string) string {
	hw, err := net.ParseMAC(mac)
	if err != nil {
		var macPieces []string
		currentPiece := ""
		for i, r := range mac {
			currentPiece = fmt.Sprintf("%s%c", currentPiece, r)
			if i%2 == 1 {
				macPieces = append(macPieces, currentPiece)
				currentPiece = ""
			}
		}

		mac = strings.Join(macPieces, ":")

		hw, err = net.ParseMAC(mac)
		if err != nil {
			return badMAC
		}
	}

	return hw.String()
}

func getStateFromHSM() *SMData {
	if smClient != nil {
		var headers map[string][]string
		authEnabled, err := TestSMAuthEnabled(authRetryCount, authRetryWait)
		if err != nil {
			log.Printf("Failed to test if SM auth is enabled: %v")
			return nil
		}
		if authEnabled {
			err = smClient.JWTTestAndRefresh()
			if err != nil {
				log.Printf("Failed to refresh JWT: %v", err)
				return nil
			}
			headers = map[string][]string{
				"Authorization": {"Bearer " + accessToken},
			}
		}
		log.Printf("Retrieving state info from %s", smBaseURL)
		url := smBaseURL + "/State/Components?type=Node"
		debugf("url: %s, smClient: %v\n", url, smClient)
		req, rerr := http.NewRequest(http.MethodGet, url, nil)
		if rerr != nil {
			log.Printf("Failed to create HTTP request for '%s': %v", url, rerr)
			return nil
		}
		if authEnabled {
			req.Header = headers
		}
		req.Close = true
		base.SetHTTPUserAgent(req, serviceName)
		r, err := smClient.Do(req)
		if err != nil {
			log.Printf("Sm State request %s failed: %v", url, err)
			return nil
		}
		debugf("getStateFromHSM(): GET %s -> r: %v, err: %v\n", url, r, err)
		var comps SMData
		err = json.NewDecoder(r.Body).Decode(&comps)
		r.Body.Close()
		// Set up an indexing map to speed up lookup of components in the list
		compsIndex := make(map[string]int, len(comps.Components))
		for i, c := range comps.Components {
			compsIndex[c.ID] = i
		}

		url = smBaseURL + "/Inventory/ComponentEndpoints?type=Node"
		req, rerr = http.NewRequest(http.MethodGet, url, nil)
		if rerr != nil {
			log.Printf("Failed to create HTTP request for '%s': %v", url, rerr)
			return nil
		}
		if authEnabled {
			req.Header = headers
		}
		req.Close = true
		base.SetHTTPUserAgent(req, serviceName)
		r, err = smClient.Do(req)
		if err != nil {
			log.Printf("Sm Inventory request %s failed: %v", url, err)
			return nil
		}
		debugf("getStateFromHSM(): GET %s -> r: %v, err: %v\n", url, r, err)
		var ep sm.ComponentEndpointArray
		ce, err := ioutil.ReadAll(r.Body)
		err = json.Unmarshal(ce, &ep)
		debugf("getStateFromHSM(): GET %s -> r: %v, err: %v\n", url, r, err)
		r.Body.Close()

		type myCompEndpt struct {
			ID           string `json:"ID"`
			Enabled      *bool  `json:"Enabled"`
			RfEndpointID string `json: "RedfishEndpointID"`
		}
		type myCompEndptArray struct {
			CompEndpts []*myCompEndpt `json:"ComponentEndpoints"`
		}
		var mep myCompEndptArray
		if err == nil {
			err = json.Unmarshal(ce, &mep)
		}

		// We use a map rather than a list.  The values in the map don't matter,
		// just the keys.  This way duplicates get filtered out.  We will most
		// likely have duplicates in the Redfish Endpoint IDs.
		cMap := make(map[string]bool)
		for idx, e := range ep.ComponentEndpoints {
			debugf("Endpoint: %v\n", e)
			if cIndex, gotIt := compsIndex[e.ID]; gotIt {
				comps.Components[cIndex].Fqdn = e.FQDN
				if e.MACAddr != "" && !strings.EqualFold(e.MACAddr, badMAC) &&
					!strings.EqualFold(e.MACAddr, undefinedMAC) {
					comps.Components[cIndex].Mac = append(comps.Components[cIndex].Mac, e.MACAddr)
				}
				if mep.CompEndpts[idx].Enabled != nil {
					debugf("%s: Enable: %s", e.ID, *mep.CompEndpts[idx].Enabled)
					comps.Components[cIndex].EndpointEnabled = *mep.CompEndpts[idx].Enabled
				} else {
					debugf("%s: Enable: nil (true)", e.ID)
					comps.Components[cIndex].EndpointEnabled = true
				}
				switch e.ComponentEndpointType {
				case sm.CompEPTypeSystem:
					getMacs(&comps.Components[cIndex], e.RedfishSystemInfo.EthNICInfo)
				case sm.CompEPTypeManager:
					getMacs(&comps.Components[cIndex], e.RedfishManagerInfo.EthNICInfo)
				case sm.CompEPTypeChassis:
					// Nothing
				}
				if e.RfEndpointID != "" {
					cMap[e.RfEndpointID] = true
				}
			}
		}

		//ip address
		url = smBaseURL + "/Inventory/EthernetInterfaces?type=Node"
		req, rerr = http.NewRequest(http.MethodGet, url, nil)
		if rerr != nil {
			log.Printf("Failed to create HTTP request for '%s': %v", url, rerr)
			return nil
		}
		if authEnabled {
			req.Header = headers
		}
		req.Close = true
		base.SetHTTPUserAgent(req, serviceName)
		r, err = smClient.Do(req)
		if err != nil {
			log.Printf("Sm Inventory request %s failed: %v", url, err)
			return nil
		}
		debugf("getStateFromHSM(): GET %s -> r: %v, err: %v\n", url, r, err)

		var ethIfaces []sm.CompEthInterfaceV2

		ce, err = ioutil.ReadAll(r.Body)
		err = json.Unmarshal(ce, &ethIfaces)
		r.Body.Close()

		addresses := make(map[string]sm.CompEthInterfaceV2)
		for _, e := range ethIfaces {
			debugf("EthInterface: %v\n", e)
			for _, ip := range e.IPAddrs {
				if ip.IPAddr != "" {
					addresses[ip.IPAddr] = e
				}
			}

			// Also see if this EthernetInterface belongs to any Components.
			for index, _ := range comps.Components {
				component := comps.Components[index]

				if component.ID == e.CompID {
					comps.Components[index].Mac = append(comps.Components[index].Mac, ensureLegalMAC(e.MACAddr))
				}
			}
		}

		comps.IPAddrs = addresses

		// Now get a list of the keys:
		compList := make([]string, 0, len(cMap)+len(comps.Components))
		for i, c := range comps.Components {
			compList = append(compList, c.ID)
			debugf("Comp[%d]: %v\n", i, c)
		}
		// Add Redfish Endpoints to the component list for subscription to the notifier
		for k := range cMap {
			compList = append(compList, k)
		}
		notifier.subscribe(compList)
		return &comps
	}
	return nil
}

func getStateFromFile() (ret *SMData) {
	if smJSONFile != "" {
		log.Printf("Retrieving state info from %s", smJSONFile)
		debugf("Reading HSM info from %s", smJSONFile)
		f, err := os.Open(smJSONFile)
		if err != nil {
			log.Printf("Error: %v\n", err)
		} else {
			defer f.Close()
			var comps SMData
			dec := json.NewDecoder(f)
			err = dec.Decode(&comps)
			if err != nil {
				log.Printf("Error: %v\n", err)
			} else {
				ret = &comps
			}
		}
	}
	return ret
}

func getStateInfo() (ret *SMData) {
	ret = getStateFromHSM()
	if ret == nil {
		ret = getStateFromFile()
	}
	return ret
}

func protectedGetState(ts int64) (*SMData, map[string]SMComponent) {
	smMutex.Lock()
	defer smMutex.Unlock()
	if ts < 0 || ts > smTimeStamp || smData == nil {
		if ts <= 0 {
			smTimeStamp = time.Now().Unix()
		} else {
			smTimeStamp = ts
		}
		newSMData := getStateInfo()
		if newSMData != nil {
			smData = newSMData
			smDataMap = makeSmMap(smData)
		}
	}
	return smData, smDataMap
}

func getState() *SMData {
	data, _ := protectedGetState(0)
	return data
}

func getStateAndMap() (*SMData, map[string]SMComponent) {
	return protectedGetState(0)
}

func refreshState(ts int64) *SMData {
	data, _ := protectedGetState(ts)
	return data
}

func FindSMCompByMAC(mac string) (SMComponent, bool) {
	state := getState()
	for _, v := range state.Components {
		if !strings.EqualFold(v.State, "empty") {
			for _, m := range v.Mac {
				if strings.EqualFold(mac, m) {
					return v, true
				}
			}
		}
	}
	return SMComponent{}, false
}

func FindSMCompByNameInCache(host string) (SMComponent, bool) {
	_, stateMap := getStateAndMap()
	if v, ok := stateMap[host]; ok {
		return v, true
	}
	return SMComponent{}, false
}

func FindSMCompByName(host string) (SMComponent, bool) {
	debugf("Searching SM data for %s\n", host)
	state := getState()
	for i, v := range state.Components {
		debugf("SM data[%d]: %v\n", i, v)
		if v.ID == host {
			return v, true
		}
	}
	return SMComponent{}, false
}

func FindSMCompByNid(nid int) (SMComponent, bool) {
	state := getState()
	for _, v := range state.Components {
		if vnid, err := v.NID.Int64(); err == nil && vnid == int64(nid) {
			return v, true
		}
	}
	return SMComponent{}, false
}

func FindXnameByIP(ip string) (string, bool) {
	// This is how many minutes we subtract from time.Now().
	// This will cause refreshState to refresh ever `cacheEvictionTime` minutes.
	// 10 minutes was chosen to start with as it seems reasonable.
	// We need to semi-frequently refresh this data in case IP addresses change
	// due to DHCP lease expirations.
	cacheEvictionTime := 10

	currTime := time.Now()
	ts := currTime.Add(time.Duration(-cacheEvictionTime) * time.Minute)
	state := refreshState(ts.Unix())

	ethIFace, found := state.IPAddrs[ip]
	if !found {
		// If we didn't find the IP, try again with a current timestamp
		// to force getting new state from HSM. In case the hardware came up
		// within the last cache eviction period.
		state = refreshState(time.Now().Unix())
		ethIFace, found = state.IPAddrs[ip]
	}
	return ethIFace.CompID, found
}

const state_manager_data_temp = `{
    "Components": [
        { "Id" : "x0c0s0b0n0", "NID":4, "FQDN" : "x0c0s0b0n0.test.com",
          "State":"Ready", "Role":"System",
          "MAC":[  "00:1e:67:e3:46:93",  "00:1e:67:e3:46:94" ], "EndpointEnabled": true },
        { "Id" : "x0c0s1b0n0", "NID":8, "FQDN" : "x0c0s1b0n0.test.com",
          "State":"Ready", "Role":"Management",
          "MAC":[  "00:1e:67:e3:46:51",  "00:1e:67:e3:46:52" ], "EndpointEnabled": true },
        { "Id" : "x0c0s2b0n0", "NID":12,"FQDN" : "x0c0s2b0n0.test.com",
          "State":"Ready", "Role":"Compute",
          "MAC":[  "00:1e:67:df:f4:f1",  "00:1e:67:df:f4:f2" ], "EndpointEnabled": true },
        { "Id" : "x0c0s3b0n0", "NID":16,"FQDN" : "x0c0s3b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:dd:d0:00",  "00:1e:67:dd:d0:01" ], "EndpointEnabled": true },
        { "Id" : "x0c0s4b0n0", "NID":20, "FQDN" : "x0c0s4b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:df:f7:0d",  "00:1e:67:df:f7:0e" ], "EndpointEnabled": true },
        { "Id" : "x0c0s5b0n0", "NID":24, "FQDN" : "x0c0s5b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:d8:9a:e1",  "00:1e:67:d8:9a:e2" ], "EndpointEnabled": true },
        { "Id" : "x0c0s6b0n0", "NID":28, "FQDN" : "x0c0s6b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:dd:d1:27",  "00:1e:67:dd:d1:28" ], "EndpointEnabled": true },
        { "Id" : "x0c0s7b0n0", "NID":32, "FQDN" : "x0c0s7b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:dd:cd:49",  "00:1e:67:dd:cd:4a" ], "EndpointEnabled": true },
        { "Id" : "x0c0s8b0n0", "NID":36, "FQDN" : "x0c0s8b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:df:f7:26",  "00:1e:67:df:f7:27" ], "EndpointEnabled": true },
        { "Id" : "x0c0s9b0n0", "NID":40, "FQDN" : "x0c0s9b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e0:00:8b",  "00:1e:67:e0:00:8c" ], "EndpointEnabled": true },
        { "Id" : "x0c0s10b0n0","NID":44, "FQDN" : "x0c0s10b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e0:02:39",  "00:1e:67:e0:02:3a" ], "EndpointEnabled": true },
        { "Id" : "x0c0s11b0n0","NID":48, "FQDN" : "x0c0s11b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:81:ac",  "00:1e:67:e3:81:ad" ], "EndpointEnabled": true },
        { "Id" : "x0c0s12b0n0","NID":52, "FQDN" : "x0c0s12b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:46:6a",  "00:1e:67:e3:46:6b" ], "EndpointEnabled": true },
        { "Id" : "x0c0s13b0n0","NID":56, "FQDN" : "x0c0s13b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:80:03",  "00:1e:67:e3:80:04" ], "EndpointEnabled": true },
        { "Id" : "x0c0s14b0n0","NID":60, "FQDN" : "x0c0s14b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:dd:c1:46",  "00:1e:67:dd:c1:47" ], "EndpointEnabled": true },
        { "Id" : "x0c0s15b0n0","NID":64, "FQDN" : "x0c0s15b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:3e:f9",  "00:1e:67:e3:3e:fa" ], "EndpointEnabled": true },
        { "Id" : "x0c0s16b0n0","NID":68, "FQDN" : "x0c0s16b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:3e:e0",  "00:1e:67:e3:3e:e1" ], "EndpointEnabled": true },
        { "Id" : "x0c0s17b0n0","NID":72, "FQDN" : "x0c0s17b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:48:09",  "00:1e:67:e3:48:0a" ], "EndpointEnabled": true },
        { "Id" : "x0c0s18b0n0","NID":76, "FQDN" : "x0c0s18b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:dd:d1:7c",  "00:1e:67:dd:d1:7d" ], "EndpointEnabled": true },
        { "Id" : "x0c0s19b0n0","NID":80, "FQDN" : "x0c0s19b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:3b:1b",  "00:1e:67:e3:3b:1c" ], "EndpointEnabled": true },
        { "Id" : "x0c0s20b0n0","NID":84, "FQDN" : "x0c0s20b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:7d:1a",  "00:1e:67:e3:7d:1b" ], "EndpointEnabled": true },
        { "Id" : "x0c0s21b0n0","NID":88, "FQDN" : "x0c0s21b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:dd:cd:da",  "00:1e:67:dd:cd:db" ], "EndpointEnabled": true },
        { "Id" : "x0c0s22b0n0","NID":92, "FQDN" : "x0c0s22b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:82:e7",  "00:1e:67:e3:82:e8" ], "EndpointEnabled": true },
        { "Id" : "x0c0s23b0n0","NID":96, "FQDN" : "x0c0s23b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:7c:43",  "00:1e:67:e3:7c:44" ], "EndpointEnabled": true },
        { "Id" : "x0c0s24b0n0","NID":100, "FQDN" : "x0c0s24b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:3e:3b",  "00:1e:67:e3:3e:3c" ], "EndpointEnabled": true },
        { "Id" : "x0c0s25b0n0","NID":104, "FQDN" : "x0c0s25b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:39:27",  "00:1e:67:e3:39:28" ], "EndpointEnabled": true },
        { "Id" : "x0c0s26b0n0","NID":108, "FQDN" : "x0c0s26b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:df:fb:40",  "00:1e:67:df:fb:41" ], "EndpointEnabled": true },
        { "Id" : "x0c0s27b0n0","NID":112, "FQDN" : "x0c0s27b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:75:54",  "00:1e:67:e3:75:55" ], "EndpointEnabled": true },
        { "Id" : "x0c0s28b0n0","NID":116, "FQDN" : "x0c0s28b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e0:06:17",  "00:1e:67:e0:06:18" ], "EndpointEnabled": true },
        { "Id" : "x0c0s29b0n0","NID":120, "FQDN" : "x0c0s29b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:47:82",  "00:1e:67:e3:47:83" ], "EndpointEnabled": true },
        { "Id" : "x0c0s30b0n0","NID":124, "FQDN" : "x0c0s30b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:df:fd:43",  "00:1e:67:df:fd:44" ], "EndpointEnabled": true },
        { "Id" : "x0c0s31b0n0","NID":128, "FQDN" : "x0c0s31b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:dd:d1:59",  "00:1e:67:dd:d1:5a" ], "EndpointEnabled": true },
        { "Id" : "x0c1s0b0n0","NID":132, "FQDN" : "x0c0s0b0n0.test.com",
          "State":"Ready", "Role":"System",
          "MAC":[  "00:1e:67:e3:45:a2",  "00:1e:67:e3:45:a3" ], "EndpointEnabled": true },
        { "Id" : "x0c1s1b0n0","NID":136, "FQDN" : "x0c0s1b0n0.test.com",
          "State":"Ready", "Role":"Management",
          "MAC":[  "00:1e:67:e3:3f:a8",  "00:1e:67:e3:3f:a9" ], "EndpointEnabled": true },
        { "Id" : "x0c1s2b0n0","NID":140, "FQDN" : "x0c0s2b0n0.test.com",
          "State":"Ready", "Role":"Compute",
          "MAC":[  "00:1e:67:e3:7f:c2",  "00:1e:67:e3:7f:c3" ], "EndpointEnabled": true },
        { "Id" : "x0c1s3b0n0","NID":144, "FQDN" : "x0c0s3b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:dd:cd:0d",  "00:1e:67:dd:cd:0e" ], "EndpointEnabled": true },
        { "Id" : "x0c1s4b0n0","NID":148, "FQDN" : "x0c0s4b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:77:f2",  "00:1e:67:e3:77:f3" ], "EndpointEnabled": true },
        { "Id" : "x0c1s5b0n0","NID":152, "FQDN" : "x0c0s5b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:83:2d",  "00:1e:67:e3:83:2e" ], "EndpointEnabled": true },
        { "Id" : "x0c1s6b0n0","NID":156, "FQDN" : "x0c0s6b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:df:fa:a5",  "00:1e:67:df:fa:a6" ], "EndpointEnabled": true },
        { "Id" : "x0c1s7b0n0","NID":160, "FQDN" : "x0c0s7b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:dd:ca:ce",  "00:1e:67:dd:ca:cf" ], "EndpointEnabled": true },
        { "Id" : "x0c1s8b0n0","NID":164, "FQDN" : "x0c0s8b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:df:f7:df",  "00:1e:67:df:f7:e0" ], "EndpointEnabled": true },
        { "Id" : "x0c1s9b0n0","NID":168, "FQDN" : "x0c0s9b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:d8:a3:f6",  "00:1e:67:d8:a3:f7" ], "EndpointEnabled": true },
        { "Id" : "x0c1s10b0n0","NID":172, "FQDN" : "x0c0s10b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:3f:c1",  "00:1e:67:e3:3f:c2" ], "EndpointEnabled": true },
        { "Id" : "x0c1s11b0n0","NID":176, "FQDN" : "x0c0s11b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:dd:ca:b0",  "00:1e:67:dd:ca:b1" ], "EndpointEnabled": true },
        { "Id" : "x0c1s12b0n0","NID":180, "FQDN" : "x0c0s12b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:40:20",  "00:1e:67:e3:40:21" ], "EndpointEnabled": true },
        { "Id" : "x0c1s13b0n0","NID":184, "FQDN" : "x0c0s13b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:df:ff:b9",  "00:1e:67:df:ff:ba" ], "EndpointEnabled": true },
        { "Id" : "x0c1s14b0n0","NID":188, "FQDN" : "x0c0s14b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:45:e8",  "00:1e:67:e3:45:e9" ], "EndpointEnabled": true },
        { "Id" : "x0c1s15b0n0","NID":192, "FQDN" : "x0c0s15b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:41:f1",  "00:1e:67:e3:41:f2" ], "EndpointEnabled": true },
        { "Id" : "x0c1s16b0n0","NID":196, "FQDN" : "x0c0s16b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:47:14",  "00:1e:67:e3:47:15" ], "EndpointEnabled": true },
        { "Id" : "x0c1s17b0n0","NID":200, "FQDN" : "x0c0s17b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:dd:c7:d6",  "00:1e:67:dd:c7:d7" ], "EndpointEnabled": true },
        { "Id" : "x0c1s18b0n0","NID":204, "FQDN" : "x0c0s18b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:42:78",  "00:1e:67:e3:42:79" ], "EndpointEnabled": true },
        { "Id" : "x0c1s19b0n0","NID":208, "FQDN" : "x0c0s19b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:3f:85",  "00:1e:67:e3:3f:86" ], "EndpointEnabled": true },
        { "Id" : "x0c1s20b0n0","NID":212, "FQDN" : "x0c0s20b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:df:f5:be",  "00:1e:67:df:f5:bf" ], "EndpointEnabled": true },
        { "Id" : "x0c1s21b0n0","NID":216, "FQDN" : "x0c0s21b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:d6:24:ce",  "00:1e:67:d6:24:cf" ], "EndpointEnabled": true },
        { "Id" : "x0c1s22b0n0","NID":220, "FQDN" : "x0c0s22b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:df:fb:4a",  "00:1e:67:df:fb:4b" ], "EndpointEnabled": true },
        { "Id" : "x0c1s23b0n0","NID":224, "FQDN" : "x0c0s23b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:3d:be",  "00:1e:67:e3:3d:bf" ], "EndpointEnabled": true },
        { "Id" : "x0c1s24b0n0","NID":228, "FQDN" : "x0c0s24b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:dd:c1:d2",  "00:1e:67:dd:c1:d3" ], "EndpointEnabled": true },
        { "Id" : "x0c1s25b0n0","NID":232, "FQDN" : "x0c0s25b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:42:73",  "00:1e:67:e3:42:74" ], "EndpointEnabled": true },
        { "Id" : "x0c1s26b0n0","NID":236, "FQDN" : "x0c0s26b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:df:f8:70",  "00:1e:67:df:f8:71" ], "EndpointEnabled": true },
        { "Id" : "x0c1s27b0n0","NID":240, "FQDN" : "x0c0s27b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:47:dc",  "00:1e:67:e3:47:dd" ], "EndpointEnabled": true },
        { "Id" : "x0c1s28b0n0","NID":244, "FQDN" : "x0c0s28b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:3d:b9",  "00:1e:67:e3:3d:ba" ], "EndpointEnabled": true },
        { "Id" : "x0c1s29b0n0","NID":248, "FQDN" : "x0c0s29b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:d0:24:b2",  "00:1e:67:d0:24:b3" ], "EndpointEnabled": true },
        { "Id" : "x0c1s30b0n0","NID":252, "FQDN" : "x0c0s30b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:df:fb:ae",  "00:1e:67:df:fb:af" ], "EndpointEnabled": true },
        { "Id" : "x0c1s31b0n0","NID":256, "FQDN" : "x0c0s31b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:dd:ce:1b",  "00:1e:67:dd:ce:1c" ], "EndpointEnabled": true },
        { "Id" : "x0c2s0b0n0","NID":260, "FQDN" : "x0c0s64b0n0.test.com",
          "State":"Ready", "Role":"System",
          "MAC":[  "00:1e:67:dd:d3:d4",  "00:1e:67:dd:d3:d5" ], "EndpointEnabled": true },
        { "Id" : "x0c2s1b0n0","NID":264, "FQDN" : "x0c0s65b0n0.test.com",
          "State":"Ready", "Role":"Management",
          "MAC":[  "00:1e:67:e3:3f:cb",  "00:1e:67:e3:3f:cc" ], "EndpointEnabled": true },
        { "Id" : "x0c2s2b0n0","NID":268, "FQDN" : "x0c0s66b0n0.test.com",
          "State":"Ready", "Role":"Compute",
          "MAC":[  "00:1e:67:df:fa:91",  "00:1e:67:df:fa:92" ], "EndpointEnabled": true },
        { "Id" : "x0c2s3b0n0","NID":272, "FQDN" : "x0c0s67b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:3f:6c",  "00:1e:67:e3:3f:6d" ], "EndpointEnabled": true },
        { "Id" : "x0c2s4b0n0","NID":276, "FQDN" : "x0c0s68b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:42:5a",  "00:1e:67:e3:42:5b" ], "EndpointEnabled": true },
        { "Id" : "x0c2s5b0n0","NID":280, "FQDN" : "x0c0s69b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:3f:ee",  "00:1e:67:e3:3f:ef" ], "EndpointEnabled": true },
        { "Id" : "x0c2s6b0n0","NID":284, "FQDN" : "x0c0s70b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:41:b5",  "00:1e:67:e3:41:b6" ], "EndpointEnabled": true },
        { "Id" : "x0c2s7b0n0","NID":292, "FQDN" : "x0c0s71b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:df:fd:61",  "00:1e:67:df:fd:62" ], "EndpointEnabled": true },
        { "Id" : "x0c2s8b0n0","NID":296, "FQDN" : "x0c0s72b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:3f:e9",  "00:1e:67:e3:3f:ea" ], "EndpointEnabled": true },
        { "Id" : "x0c2s9b0n0","NID":300, "FQDN" : "x0c0s73b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:dd:c1:af",  "00:1e:67:dd:c1:b0" ], "EndpointEnabled": true },
        { "Id" : "x0c2s10b0n0","NID":304, "FQDN" : "x0c0s74b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:81:66",  "00:1e:67:e3:81:67" ], "EndpointEnabled": true },
        { "Id" : "x0c2s11b0n0","NID":308, "FQDN" : "x0c0s75b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:dd:c1:5f",  "00:1e:67:dd:c1:60" ], "EndpointEnabled": true },
        { "Id" : "x0c2s12b0n0","NID":312, "FQDN" : "x0c0s76b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:46:bf",  "00:1e:67:e3:46:c0" ], "EndpointEnabled": true },
        { "Id" : "x0c2s13b0n0","NID":316, "FQDN" : "x0c0s77b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:dd:d2:44",  "00:1e:67:dd:d2:45" ], "EndpointEnabled": true },
        { "Id" : "x0c2s14b0n0","NID":320, "FQDN" : "x0c0s78b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:d8:95:69",  "00:1e:67:d8:95:6a" ], "EndpointEnabled": true },
        { "Id" : "x0c2s15b0n0","NID":324, "FQDN" : "x0c0s79b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:dd:d2:9e",  "00:1e:67:dd:d2:9f" ], "EndpointEnabled": true },
        { "Id" : "x0c2s16b0n0","NID":328, "FQDN" : "x0c0s80b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:dd:d1:b3",  "00:1e:67:dd:d1:b4" ], "EndpointEnabled": true },
        { "Id" : "x0c2s17b0n0","NID":332, "FQDN" : "x0c0s81b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:46:e2",  "00:1e:67:e3:46:e3" ], "EndpointEnabled": true },
        { "Id" : "x0c2s18b0n0","NID":336, "FQDN" : "x0c0s82b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:47:6e",  "00:1e:67:e3:47:6f" ], "EndpointEnabled": true },
        { "Id" : "x0c2s19b0n0","NID":340, "FQDN" : "x0c0s83b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:3f:e4",  "00:1e:67:e3:3f:e5" ], "EndpointEnabled": true },
        { "Id" : "x0c2s20b0n0","NID":344, "FQDN" : "x0c0s84b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:dd:cb:af",  "00:1e:67:dd:cb:b0" ], "EndpointEnabled": true },
        { "Id" : "x0c2s21b0n0","NID":348, "FQDN" : "x0c0s85b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:dd:c8:80",  "00:1e:67:dd:c8:81" ], "EndpointEnabled": true },
        { "Id" : "x0c2s22b0n0","NID":352, "FQDN" : "x0c0s86b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e0:05:ea",  "00:1e:67:e0:05:eb" ], "EndpointEnabled": true },
        { "Id" : "x0c2s23b0n0","NID":356, "FQDN" : "x0c0s87b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:d8:a3:10",  "00:1e:67:d8:a3:11" ], "EndpointEnabled": true },
        { "Id" : "x0c2s24b0n0","NID":360, "FQDN" : "x0c0s88b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:3d:a5",  "00:1e:67:e3:3d:a6" ], "EndpointEnabled": true },
        { "Id" : "x0c2s25b0n0","NID":364, "FQDN" : "x0c0s89b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:dd:c2:db",  "00:1e:67:dd:c2:dc" ], "EndpointEnabled": true },
        { "Id" : "x0c2s26b0n0","NID":368, "FQDN" : "x0c0s90b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:d8:a4:87",  "00:1e:67:d8:a4:88" ], "EndpointEnabled": true },
        { "Id" : "x0c2s27b0n0","NID":372, "FQDN" : "x0c0s91b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:df:fa:c3",  "00:1e:67:df:fa:c4" ], "EndpointEnabled": true },
        { "Id" : "x0c2s28b0n0","NID":376, "FQDN" : "x0c0s92b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:3f:17",  "00:1e:67:e3:3f:18" ], "EndpointEnabled": true },
        { "Id" : "x0c2s29b0n0","NID":380, "FQDN" : "x0c0s93b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:46:a6",  "00:1e:67:e3:46:a7" ], "EndpointEnabled": true },
        { "Id" : "x0c2s30b0n0","NID":384, "FQDN" : "x0c0s94b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:df:fa:9b",  "00:1e:67:df:fa:9c" ], "EndpointEnabled": true },
        { "Id" : "x0c2s31b0n0","NID":388, "FQDN" : "x0c0s95b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:dd:c9:7f",  "00:1e:67:dd:c9:80" ], "EndpointEnabled": true },
        { "Id" : "x0c3s0b0n0","NID":392, "FQDN" : "x0c3s0b0n0.test.com",
          "State":"Ready", "Role":"Storage",
          "MAC":[  "00:1e:67:e3:40:11",  "00:1e:67:e3:40:12" ], "EndpointEnabled": true },
        { "Id" : "x0c3s1b0n0","NID":396, "FQDN" : "x0c3s1b0n0.test.com",
          "State":"Ready", "Role":"Management",
          "MAC":[  "00:1e:67:e3:3e:2c",  "00:1e:67:e3:3e:2d" ], "EndpointEnabled": true },
        { "Id" : "x0c3s2b0n0","NID":400, "FQDN" : "x0c3s2b0n0.test.com",
          "State":"Ready", "Role":"Compute",
          "MAC":[  "00:1e:67:e3:3f:58",  "00:1e:67:e3:3f:59" ], "EndpointEnabled": true },
        { "Id" : "x0c3s3b0n0","NID":404, "FQDN" : "x0c3s3b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:47:05",  "00:1e:67:e3:47:06" ], "EndpointEnabled": true },
        { "Id" : "x0c3s4b0n0","NID":408, "FQDN" : "x0c3s4b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:46:15",  "00:1e:67:e3:46:16" ], "EndpointEnabled": true },
        { "Id" : "x0c3s5b0n0","NID":412, "FQDN" : "x0c3s5b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:dd:d0:eb",  "00:1e:67:dd:d0:ec" ], "EndpointEnabled": true },
        { "Id" : "x0c3s6b0n0","NID":416, "FQDN" : "x0c3s6b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:df:f7:fe",  "00:1e:67:df:f7:ff" ], "EndpointEnabled": true },
        { "Id" : "x0c3s7b0n0","NID":420, "FQDN" : "x0c3s7b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:d8:91:22",  "00:1e:67:d8:91:23" ], "EndpointEnabled": true },
        { "Id" : "x0c3s8b0n0","NID":424, "FQDN" : "x0c3s8b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:df:f5:1e",  "00:1e:67:df:f5:1f" ], "EndpointEnabled": true },
        { "Id" : "x0c3s9b0n0","NID":428, "FQDN" : "x0c3s9b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:df:fd:34",  "00:1e:67:df:fd:35" ], "EndpointEnabled": true },
        { "Id" : "x0c3s10b0n0","NID":432, "FQDN" : "x0c3s10b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:42:05",  "00:1e:67:e3:42:06" ], "EndpointEnabled": true },
        { "Id" : "x0c3s11b0n0","NID":436, "FQDN" : "x0c3s11b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:3a:d0",  "00:1e:67:e3:3a:d1" ], "EndpointEnabled": true },
        { "Id" : "x0c3s12b0n0","NID":440, "FQDN" : "x0c3s12b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:c8:10:ab",  "00:1e:67:c8:10:ac" ], "EndpointEnabled": true },
        { "Id" : "x0c3s13b0n0","NID":444, "FQDN" : "x0c3s13b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:df:f6:77",  "00:1e:67:df:f6:78" ], "EndpointEnabled": true },
        { "Id" : "x0c3s14b0n0","NID":448, "FQDN" : "x0c3s14b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:dd:cc:09",  "00:1e:67:dd:cc:0a" ], "EndpointEnabled": true },
        { "Id" : "x0c3s15b0n0","NID":452, "FQDN" : "x0c3s15b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:dd:c8:85",  "00:1e:67:dd:c8:86" ], "EndpointEnabled": true },
        { "Id" : "x0c3s16b0n0","NID":456, "FQDN" : "x0c3s16b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:d8:9a:af",  "00:1e:67:d8:9a:b0" ], "EndpointEnabled": true },
        { "Id" : "x0c3s17b0n0","NID":460, "FQDN" : "x0c3s17b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:dd:d3:11",  "00:1e:67:dd:d3:12" ], "EndpointEnabled": true },
        { "Id" : "x0c3s18b0n0","NID":464, "FQDN" : "x0c3s18b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:df:fb:cc",  "00:1e:67:df:fb:cd" ], "EndpointEnabled": true },
        { "Id" : "x0c3s19b0n0","NID":468, "FQDN" : "x0c3s19b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:df:f7:bc",  "00:1e:67:df:f7:bd" ], "EndpointEnabled": true },
        { "Id" : "x0c3s20b0n0","NID":472, "FQDN" : "x0c3s20b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:dd:ce:11",  "00:1e:67:dd:ce:12" ], "EndpointEnabled": true },
        { "Id" : "x0c3s21b0n0","NID":476, "FQDN" : "x0c3s21b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:df:fd:5c",  "00:1e:67:df:fd:5d" ], "EndpointEnabled": true },
        { "Id" : "x0c3s22b0n0","NID":480, "FQDN" : "x0c3s22b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:dd:bf:ac",  "00:1e:67:dd:bf:ad" ], "EndpointEnabled": true },
        { "Id" : "x0c3s23b0n0","NID":484, "FQDN" : "x0c3s23b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:3b:98",  "00:1e:67:e3:3b:99" ], "EndpointEnabled": true },
        { "Id" : "x0c3s24b0n0","NID":488, "FQDN" : "x0c3s24b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:47:d2",  "00:1e:67:e3:47:d3" ], "EndpointEnabled": true },
        { "Id" : "x0c3s25b0n0","NID":492, "FQDN" : "x0c3s25b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:d8:9b:09",  "00:1e:67:d8:9b:0a" ], "EndpointEnabled": true },
        { "Id" : "x0c3s26b0n0","NID":496, "FQDN" : "x0c3s26b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e0:00:36",  "00:1e:67:e0:00:37" ], "EndpointEnabled": true },
        { "Id" : "x0c3s27b0n0","NID":500, "FQDN" : "x0c3s27b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:d8:41:3b",  "00:1e:67:d8:41:3c" ], "EndpointEnabled": true },
        { "Id" : "x0c3s28b0n0","NID":504, "FQDN" : "x0c3s28b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:81:a2",  "00:1e:67:e3:81:a3" ], "EndpointEnabled": true },
        { "Id" : "x0c3s29b0n0","NID":508, "FQDN" : "x0c3s29b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:3c:42",  "00:1e:67:e3:3c:43" ], "EndpointEnabled": true },
        { "Id" : "x0c3s30b0n0","NID":512, "FQDN" : "x0c3s30b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:df:fc:3f",  "00:1e:67:df:fc:40" ], "EndpointEnabled": true },
        { "Id" : "x0c3s31b0n0","NID":516, "FQDN" : "x0c3s31b0n0.test.com",
          "State":"Ready",
          "MAC":[  "00:1e:67:e3:46:d8",  "00:1e:67:e3:46:d9" ], "EndpointEnabled": true },
        { "Id" : "x1c0s0b0n0", "NID":1004, "FQDN" : "x1c0s0b0n0.test.com",
          "State":"Ready", "Role":"Storage",
          "MAC":[  "01:1e:67:e3:46:93",  "01:1e:67:e3:46:94" ], "EndpointEnabled": true },
        { "Id" : "x1c0s1b0n0", "NID":1008, "FQDN" : "x1c0s1b0n0.test.com",
          "State":"Ready", "Role":"Management",
          "MAC":[  "01:1e:67:e3:46:51",  "01:1e:67:e3:46:52" ], "EndpointEnabled": true },
        { "Id" : "x1c0s2b0n0", "NID":1012,"FQDN" : "x1c0s2b0n0.test.com",
          "State":"Ready", "Role":"Compute",
          "MAC":[  "01:1e:67:df:f4:f1",  "01:1e:67:df:f4:f2" ], "EndpointEnabled": true },
        { "Id" : "x1c0s3b0n0", "NID":1016,"FQDN" : "x1c0s3b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:dd:d0:00",  "01:1e:67:dd:d0:01" ], "EndpointEnabled": true },
        { "Id" : "x1c0s4b0n0", "NID":1020, "FQDN" : "x1c0s4b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:df:f7:0d",  "01:1e:67:df:f7:0e" ], "EndpointEnabled": true },
        { "Id" : "x1c0s5b0n0", "NID":1024, "FQDN" : "x1c0s5b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:d8:9a:e1",  "01:1e:67:d8:9a:e2" ], "EndpointEnabled": true },
        { "Id" : "x1c0s6b0n0", "NID":1028, "FQDN" : "x1c0s6b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:dd:d1:27",  "01:1e:67:dd:d1:28" ], "EndpointEnabled": true },
        { "Id" : "x1c0s7b0n0", "NID":1032, "FQDN" : "x1c0s7b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:dd:cd:49",  "01:1e:67:dd:cd:4a" ], "EndpointEnabled": true },
        { "Id" : "x1c0s8b0n0", "NID":1036, "FQDN" : "x1c0s8b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:df:f7:26",  "01:1e:67:df:f7:27" ], "EndpointEnabled": true },
        { "Id" : "x1c0s9b0n0", "NID":1040, "FQDN" : "x1c0s9b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e0:00:8b",  "01:1e:67:e0:00:8c" ], "EndpointEnabled": true },
        { "Id" : "x1c0s10b0n0","NID":1044, "FQDN" : "x1c0s10b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e0:02:39",  "01:1e:67:e0:02:3a" ], "EndpointEnabled": true },
        { "Id" : "x1c0s11b0n0","NID":1048, "FQDN" : "x1c0s11b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:81:ac",  "01:1e:67:e3:81:ad" ], "EndpointEnabled": true },
        { "Id" : "x1c0s12b0n0","NID":1052, "FQDN" : "x1c0s12b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:46:6a",  "01:1e:67:e3:46:6b" ], "EndpointEnabled": true },
        { "Id" : "x1c0s13b0n0","NID":1056, "FQDN" : "x1c0s13b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:80:03",  "01:1e:67:e3:80:04" ], "EndpointEnabled": true },
        { "Id" : "x1c0s14b0n0","NID":1060, "FQDN" : "x1c0s14b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:dd:c1:46",  "01:1e:67:dd:c1:47" ], "EndpointEnabled": true },
        { "Id" : "x1c0s15b0n0","NID":1064, "FQDN" : "x1c0s15b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:3e:f9",  "01:1e:67:e3:3e:fa" ], "EndpointEnabled": true },
        { "Id" : "x1c0s16b0n0","NID":1068, "FQDN" : "x1c0s16b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:3e:e0",  "01:1e:67:e3:3e:e1" ], "EndpointEnabled": true },
        { "Id" : "x1c0s17b0n0","NID":1072, "FQDN" : "x1c0s17b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:48:09",  "01:1e:67:e3:48:0a" ], "EndpointEnabled": true },
        { "Id" : "x1c0s18b0n0","NID":1076, "FQDN" : "x1c0s18b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:dd:d1:7c",  "01:1e:67:dd:d1:7d" ], "EndpointEnabled": true },
        { "Id" : "x1c0s19b0n0","NID":1080, "FQDN" : "x1c0s19b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:3b:1b",  "01:1e:67:e3:3b:1c" ], "EndpointEnabled": true },
        { "Id" : "x1c0s20b0n0","NID":1084, "FQDN" : "x1c0s20b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:7d:1a",  "01:1e:67:e3:7d:1b" ], "EndpointEnabled": true },
        { "Id" : "x1c0s21b0n0","NID":1088, "FQDN" : "x1c0s21b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:dd:cd:da",  "01:1e:67:dd:cd:db" ], "EndpointEnabled": true },
        { "Id" : "x1c0s22b0n0","NID":1092, "FQDN" : "x1c0s22b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:82:e7",  "01:1e:67:e3:82:e8" ], "EndpointEnabled": true },
        { "Id" : "x1c0s23b0n0","NID":1096, "FQDN" : "x1c0s23b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:7c:43",  "01:1e:67:e3:7c:44" ], "EndpointEnabled": true },
        { "Id" : "x1c0s24b0n0","NID":1100, "FQDN" : "x1c0s24b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:3e:3b",  "01:1e:67:e3:3e:3c" ], "EndpointEnabled": true },
        { "Id" : "x1c0s25b0n0","NID":1104, "FQDN" : "x1c0s25b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:39:27",  "01:1e:67:e3:39:28" ], "EndpointEnabled": true },
        { "Id" : "x1c0s26b0n0","NID":1108, "FQDN" : "x1c0s26b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:df:fb:40",  "01:1e:67:df:fb:41" ], "EndpointEnabled": true },
        { "Id" : "x1c0s27b0n0","NID":1112, "FQDN" : "x1c0s27b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:75:54",  "01:1e:67:e3:75:55" ], "EndpointEnabled": true },
        { "Id" : "x1c0s28b0n0","NID":1116, "FQDN" : "x1c0s28b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e0:06:17",  "01:1e:67:e0:06:18" ], "EndpointEnabled": true },
        { "Id" : "x1c0s29b0n0","NID":1120, "FQDN" : "x1c0s29b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:47:82",  "01:1e:67:e3:47:83" ], "EndpointEnabled": true },
        { "Id" : "x1c0s30b0n0","NID":1124, "FQDN" : "x1c0s30b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:df:fd:43",  "01:1e:67:df:fd:44" ], "EndpointEnabled": true },
        { "Id" : "x1c0s31b0n0","NID":1128, "FQDN" : "x1c0s31b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:dd:d1:59",  "01:1e:67:dd:d1:5a" ], "EndpointEnabled": true },
        { "Id" : "x1c1s0b0n0","NID":1132, "FQDN" : "x1c0s0b0n0.test.com",
          "State":"Ready", "Role":"Storage",
          "MAC":[  "01:1e:67:e3:45:a2",  "01:1e:67:e3:45:a3" ], "EndpointEnabled": true },
        { "Id" : "x1c1s1b0n0","NID":1136, "FQDN" : "x1c0s1b0n0.test.com",
          "State":"Ready", "Role":"Management",
          "MAC":[  "01:1e:67:e3:3f:a8",  "01:1e:67:e3:3f:a9" ], "EndpointEnabled": true },
        { "Id" : "x1c1s2b0n0","NID":1140, "FQDN" : "x1c0s2b0n0.test.com",
          "State":"Ready", "Role":"Compute",
          "MAC":[  "01:1e:67:e3:7f:c2",  "01:1e:67:e3:7f:c3" ], "EndpointEnabled": true },
        { "Id" : "x1c1s3b0n0","NID":1144, "FQDN" : "x1c0s3b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:dd:cd:0d",  "01:1e:67:dd:cd:0e" ], "EndpointEnabled": true },
        { "Id" : "x1c1s4b0n0","NID":1148, "FQDN" : "x1c0s4b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:77:f2",  "01:1e:67:e3:77:f3" ], "EndpointEnabled": true },
        { "Id" : "x1c1s5b0n0","NID":1152, "FQDN" : "x1c0s5b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:83:2d",  "01:1e:67:e3:83:2e" ], "EndpointEnabled": true },
        { "Id" : "x1c1s6b0n0","NID":1156, "FQDN" : "x1c0s6b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:df:fa:a5",  "01:1e:67:df:fa:a6" ], "EndpointEnabled": true },
        { "Id" : "x1c1s7b0n0","NID":1160, "FQDN" : "x1c0s7b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:dd:ca:ce",  "01:1e:67:dd:ca:cf" ], "EndpointEnabled": true },
        { "Id" : "x1c1s8b0n0","NID":1164, "FQDN" : "x1c0s8b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:df:f7:df",  "01:1e:67:df:f7:e0" ], "EndpointEnabled": true },
        { "Id" : "x1c1s9b0n0","NID":1168, "FQDN" : "x1c0s9b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:d8:a3:f6",  "01:1e:67:d8:a3:f7" ], "EndpointEnabled": true },
        { "Id" : "x1c1s10b0n0","NID":1172, "FQDN" : "x1c0s10b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:3f:c1",  "01:1e:67:e3:3f:c2" ], "EndpointEnabled": true },
        { "Id" : "x1c1s11b0n0","NID":1176, "FQDN" : "x1c0s11b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:dd:ca:b0",  "01:1e:67:dd:ca:b1" ], "EndpointEnabled": true },
        { "Id" : "x1c1s12b0n0","NID":1180, "FQDN" : "x1c0s12b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:40:20",  "01:1e:67:e3:40:21" ], "EndpointEnabled": true },
        { "Id" : "x1c1s13b0n0","NID":1184, "FQDN" : "x1c0s13b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:df:ff:b9",  "01:1e:67:df:ff:ba" ], "EndpointEnabled": true },
        { "Id" : "x1c1s14b0n0","NID":1188, "FQDN" : "x1c0s14b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:45:e8",  "01:1e:67:e3:45:e9" ], "EndpointEnabled": true },
        { "Id" : "x1c1s15b0n0","NID":1192, "FQDN" : "x1c0s15b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:41:f1",  "01:1e:67:e3:41:f2" ], "EndpointEnabled": true },
        { "Id" : "x1c1s16b0n0","NID":1196, "FQDN" : "x1c0s16b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:47:14",  "01:1e:67:e3:47:15" ], "EndpointEnabled": true },
        { "Id" : "x1c1s17b0n0","NID":1200, "FQDN" : "x1c0s17b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:dd:c7:d6",  "01:1e:67:dd:c7:d7" ], "EndpointEnabled": true },
        { "Id" : "x1c1s18b0n0","NID":1204, "FQDN" : "x1c0s18b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:42:78",  "01:1e:67:e3:42:79" ], "EndpointEnabled": true },
        { "Id" : "x1c1s19b0n0","NID":1208, "FQDN" : "x1c0s19b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:3f:85",  "01:1e:67:e3:3f:86" ], "EndpointEnabled": true },
        { "Id" : "x1c1s20b0n0","NID":1212, "FQDN" : "x1c0s20b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:df:f5:be",  "01:1e:67:df:f5:bf" ], "EndpointEnabled": true },
        { "Id" : "x1c1s21b0n0","NID":1216, "FQDN" : "x1c0s21b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:d6:24:ce",  "01:1e:67:d6:24:cf" ], "EndpointEnabled": true },
        { "Id" : "x1c1s22b0n0","NID":1220, "FQDN" : "x1c0s22b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:df:fb:4a",  "01:1e:67:df:fb:4b" ], "EndpointEnabled": true },
        { "Id" : "x1c1s23b0n0","NID":1224, "FQDN" : "x1c0s23b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:3d:be",  "01:1e:67:e3:3d:bf" ], "EndpointEnabled": true },
        { "Id" : "x1c1s24b0n0","NID":1228, "FQDN" : "x1c0s24b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:dd:c1:d2",  "01:1e:67:dd:c1:d3" ], "EndpointEnabled": true },
        { "Id" : "x1c1s25b0n0","NID":1232, "FQDN" : "x1c0s25b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:42:73",  "01:1e:67:e3:42:74" ], "EndpointEnabled": true },
        { "Id" : "x1c1s26b0n0","NID":1236, "FQDN" : "x1c0s26b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:df:f8:70",  "01:1e:67:df:f8:71" ], "EndpointEnabled": true },
        { "Id" : "x1c1s27b0n0","NID":1240, "FQDN" : "x1c0s27b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:47:dc",  "01:1e:67:e3:47:dd" ], "EndpointEnabled": true },
        { "Id" : "x1c1s28b0n0","NID":1244, "FQDN" : "x1c0s28b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:3d:b9",  "01:1e:67:e3:3d:ba" ], "EndpointEnabled": true },
        { "Id" : "x1c1s29b0n0","NID":1248, "FQDN" : "x1c0s29b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:d0:24:b2",  "01:1e:67:d0:24:b3" ], "EndpointEnabled": true },
        { "Id" : "x1c1s30b0n0","NID":1252, "FQDN" : "x1c0s30b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:df:fb:ae",  "01:1e:67:df:fb:af" ], "EndpointEnabled": true },
        { "Id" : "x1c1s31b0n0","NID":1256, "FQDN" : "x1c0s31b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:dd:ce:1b",  "01:1e:67:dd:ce:1c" ], "EndpointEnabled": true },
        { "Id" : "x1c2s0b0n0","NID":1260, "FQDN" : "x1c0s64b0n0.test.com",
          "State":"Ready", "Role":"Storage",
          "MAC":[  "01:1e:67:dd:d3:d4",  "01:1e:67:dd:d3:d5" ], "EndpointEnabled": true },
        { "Id" : "x1c2s1b0n0","NID":1264, "FQDN" : "x1c0s65b0n0.test.com",
          "State":"Ready", "Role":"Management",
          "MAC":[  "01:1e:67:e3:3f:cb",  "01:1e:67:e3:3f:cc" ], "EndpointEnabled": true },
        { "Id" : "x1c2s2b0n0","NID":1268, "FQDN" : "x1c0s66b0n0.test.com",
          "State":"Ready", "Role":"Compute",
          "MAC":[  "01:1e:67:df:fa:91",  "01:1e:67:df:fa:92" ], "EndpointEnabled": true },
        { "Id" : "x1c2s3b0n0","NID":1272, "FQDN" : "x1c0s67b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:3f:6c",  "01:1e:67:e3:3f:6d" ], "EndpointEnabled": true },
        { "Id" : "x1c2s4b0n0","NID":1276, "FQDN" : "x1c0s68b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:42:5a",  "01:1e:67:e3:42:5b" ], "EndpointEnabled": true },
        { "Id" : "x1c2s5b0n0","NID":1280, "FQDN" : "x1c0s69b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:3f:ee",  "01:1e:67:e3:3f:ef" ], "EndpointEnabled": true },
        { "Id" : "x1c2s6b0n0","NID":1284, "FQDN" : "x1c0s70b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:41:b5",  "01:1e:67:e3:41:b6" ], "EndpointEnabled": true },
        { "Id" : "x1c2s7b0n0","NID":1292, "FQDN" : "x1c0s71b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:df:fd:61",  "01:1e:67:df:fd:62" ], "EndpointEnabled": true },
        { "Id" : "x1c2s8b0n0","NID":1296, "FQDN" : "x1c0s72b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:3f:e9",  "01:1e:67:e3:3f:ea" ], "EndpointEnabled": true },
        { "Id" : "x1c2s9b0n0","NID":1300, "FQDN" : "x1c0s73b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:dd:c1:af",  "01:1e:67:dd:c1:b0" ], "EndpointEnabled": true },
        { "Id" : "x1c2s10b0n0","NID":1304, "FQDN" : "x1c0s74b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:81:66",  "01:1e:67:e3:81:67" ], "EndpointEnabled": true },
        { "Id" : "x1c2s11b0n0","NID":1308, "FQDN" : "x1c0s75b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:dd:c1:5f",  "01:1e:67:dd:c1:60" ], "EndpointEnabled": true },
        { "Id" : "x1c2s12b0n0","NID":1312, "FQDN" : "x1c0s76b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:46:bf",  "01:1e:67:e3:46:c0" ], "EndpointEnabled": true },
        { "Id" : "x1c2s13b0n0","NID":1316, "FQDN" : "x1c0s77b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:dd:d2:44",  "01:1e:67:dd:d2:45" ], "EndpointEnabled": true },
        { "Id" : "x1c2s14b0n0","NID":1320, "FQDN" : "x1c0s78b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:d8:95:69",  "01:1e:67:d8:95:6a" ], "EndpointEnabled": true },
        { "Id" : "x1c2s15b0n0","NID":1324, "FQDN" : "x1c0s79b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:dd:d2:9e",  "01:1e:67:dd:d2:9f" ], "EndpointEnabled": true },
        { "Id" : "x1c2s16b0n0","NID":1328, "FQDN" : "x1c0s80b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:dd:d1:b3",  "01:1e:67:dd:d1:b4" ], "EndpointEnabled": true },
        { "Id" : "x1c2s17b0n0","NID":1332, "FQDN" : "x1c0s81b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:46:e2",  "01:1e:67:e3:46:e3" ], "EndpointEnabled": true },
        { "Id" : "x1c2s18b0n0","NID":1336, "FQDN" : "x1c0s82b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:47:6e",  "01:1e:67:e3:47:6f" ], "EndpointEnabled": true },
        { "Id" : "x1c2s19b0n0","NID":1340, "FQDN" : "x1c0s83b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:3f:e4",  "01:1e:67:e3:3f:e5" ], "EndpointEnabled": true },
        { "Id" : "x1c2s20b0n0","NID":1344, "FQDN" : "x1c0s84b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:dd:cb:af",  "01:1e:67:dd:cb:b0" ], "EndpointEnabled": true },
        { "Id" : "x1c2s21b0n0","NID":1348, "FQDN" : "x1c0s85b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:dd:c8:80",  "01:1e:67:dd:c8:81" ], "EndpointEnabled": true },
        { "Id" : "x1c2s22b0n0","NID":1352, "FQDN" : "x1c0s86b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e0:05:ea",  "01:1e:67:e0:05:eb" ], "EndpointEnabled": true },
        { "Id" : "x1c2s23b0n0","NID":1356, "FQDN" : "x1c0s87b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:d8:a3:10",  "01:1e:67:d8:a3:11" ], "EndpointEnabled": true },
        { "Id" : "x1c2s24b0n0","NID":1360, "FQDN" : "x1c0s88b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:3d:a5",  "01:1e:67:e3:3d:a6" ], "EndpointEnabled": true },
        { "Id" : "x1c2s25b0n0","NID":1364, "FQDN" : "x1c0s89b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:dd:c2:db",  "01:1e:67:dd:c2:dc" ], "EndpointEnabled": true },
        { "Id" : "x1c2s26b0n0","NID":1368, "FQDN" : "x1c0s90b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:d8:a4:87",  "01:1e:67:d8:a4:88" ], "EndpointEnabled": true },
        { "Id" : "x1c2s27b0n0","NID":1372, "FQDN" : "x1c0s91b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:df:fa:c3",  "01:1e:67:df:fa:c4" ], "EndpointEnabled": true },
        { "Id" : "x1c2s28b0n0","NID":1376, "FQDN" : "x1c0s92b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:3f:17",  "01:1e:67:e3:3f:18" ], "EndpointEnabled": true },
        { "Id" : "x1c2s29b0n0","NID":1380, "FQDN" : "x1c0s93b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:46:a6",  "01:1e:67:e3:46:a7" ], "EndpointEnabled": true },
        { "Id" : "x1c2s30b0n0","NID":1384, "FQDN" : "x1c0s94b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:df:fa:9b",  "01:1e:67:df:fa:9c" ], "EndpointEnabled": true },
        { "Id" : "x1c2s31b0n0","NID":1388, "FQDN" : "x1c0s95b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:dd:c9:7f",  "01:1e:67:dd:c9:80" ], "EndpointEnabled": true },
        { "Id" : "x1c3s0b0n0","NID":1392, "FQDN" : "x1c3s0b0n0.test.com",
          "State":"Ready", "Role":"Storage",
          "MAC":[  "01:1e:67:e3:40:11",  "01:1e:67:e3:40:12" ], "EndpointEnabled": true },
        { "Id" : "x1c3s1b0n0","NID":1396, "FQDN" : "x1c3s1b0n0.test.com",
          "State":"Ready", "Role":"Management",
          "MAC":[  "01:1e:67:e3:3e:2c",  "01:1e:67:e3:3e:2d" ], "EndpointEnabled": true },
        { "Id" : "x1c3s2b0n0","NID":1400, "FQDN" : "x1c3s2b0n0.test.com",
          "State":"Ready", "Role":"Compute",
          "MAC":[  "01:1e:67:e3:3f:58",  "01:1e:67:e3:3f:59" ], "EndpointEnabled": true },
        { "Id" : "x1c3s3b0n0","NID":1404, "FQDN" : "x1c3s3b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:47:05",  "01:1e:67:e3:47:06" ], "EndpointEnabled": true },
        { "Id" : "x1c3s4b0n0","NID":1408, "FQDN" : "x1c3s4b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:46:15",  "01:1e:67:e3:46:16" ], "EndpointEnabled": true },
        { "Id" : "x1c3s5b0n0","NID":1412, "FQDN" : "x1c3s5b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:dd:d0:eb",  "01:1e:67:dd:d0:ec" ], "EndpointEnabled": true },
        { "Id" : "x1c3s6b0n0","NID":1416, "FQDN" : "x1c3s6b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:df:f7:fe",  "01:1e:67:df:f7:ff" ], "EndpointEnabled": true },
        { "Id" : "x1c3s7b0n0","NID":1420, "FQDN" : "x1c3s7b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:d8:91:22",  "01:1e:67:d8:91:23" ], "EndpointEnabled": true },
        { "Id" : "x1c3s8b0n0","NID":1424, "FQDN" : "x1c3s8b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:df:f5:1e",  "01:1e:67:df:f5:1f" ], "EndpointEnabled": true },
        { "Id" : "x1c3s9b0n0","NID":1428, "FQDN" : "x1c3s9b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:df:fd:34",  "01:1e:67:df:fd:35" ], "EndpointEnabled": true },
        { "Id" : "x1c3s10b0n0","NID":1432, "FQDN" : "x1c3s10b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:42:05",  "01:1e:67:e3:42:06" ], "EndpointEnabled": true },
        { "Id" : "x1c3s11b0n0","NID":1436, "FQDN" : "x1c3s11b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:3a:d0",  "01:1e:67:e3:3a:d1" ], "EndpointEnabled": true },
        { "Id" : "x1c3s12b0n0","NID":1440, "FQDN" : "x1c3s12b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:c8:10:ab",  "01:1e:67:c8:10:ac" ], "EndpointEnabled": true },
        { "Id" : "x1c3s13b0n0","NID":1444, "FQDN" : "x1c3s13b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:df:f6:77",  "01:1e:67:df:f6:78" ], "EndpointEnabled": true },
        { "Id" : "x1c3s14b0n0","NID":1448, "FQDN" : "x1c3s14b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:dd:cc:09",  "01:1e:67:dd:cc:0a" ], "EndpointEnabled": true },
        { "Id" : "x1c3s15b0n0","NID":1452, "FQDN" : "x1c3s15b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:dd:c8:85",  "01:1e:67:dd:c8:86" ], "EndpointEnabled": true },
        { "Id" : "x1c3s16b0n0","NID":1456, "FQDN" : "x1c3s16b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:d8:9a:af",  "01:1e:67:d8:9a:b0" ], "EndpointEnabled": true },
        { "Id" : "x1c3s17b0n0","NID":1460, "FQDN" : "x1c3s17b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:dd:d3:11",  "01:1e:67:dd:d3:12" ], "EndpointEnabled": true },
        { "Id" : "x1c3s18b0n0","NID":1464, "FQDN" : "x1c3s18b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:df:fb:cc",  "01:1e:67:df:fb:cd" ], "EndpointEnabled": true },
        { "Id" : "x1c3s19b0n0","NID":1468, "FQDN" : "x1c3s19b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:df:f7:bc",  "01:1e:67:df:f7:bd" ], "EndpointEnabled": true },
        { "Id" : "x1c3s20b0n0","NID":1472, "FQDN" : "x1c3s20b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:dd:ce:11",  "01:1e:67:dd:ce:12" ], "EndpointEnabled": true },
        { "Id" : "x1c3s21b0n0","NID":1476, "FQDN" : "x1c3s21b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:df:fd:5c",  "01:1e:67:df:fd:5d" ], "EndpointEnabled": true },
        { "Id" : "x1c3s22b0n0","NID":1480, "FQDN" : "x1c3s22b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:dd:bf:ac",  "01:1e:67:dd:bf:ad" ], "EndpointEnabled": true },
        { "Id" : "x1c3s23b0n0","NID":1484, "FQDN" : "x1c3s23b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:3b:98",  "01:1e:67:e3:3b:99" ], "EndpointEnabled": true },
        { "Id" : "x1c3s24b0n0","NID":1488, "FQDN" : "x1c3s24b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:47:d2",  "01:1e:67:e3:47:d3" ], "EndpointEnabled": true },
        { "Id" : "x1c3s25b0n0","NID":1492, "FQDN" : "x1c3s25b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:d8:9b:09",  "01:1e:67:d8:9b:0a" ], "EndpointEnabled": true },
        { "Id" : "x1c3s26b0n0","NID":1496, "FQDN" : "x1c3s26b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e0:00:36",  "01:1e:67:e0:00:37" ], "EndpointEnabled": true },
        { "Id" : "x1c3s27b0n0","NID":1500, "FQDN" : "x1c3s27b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:d8:41:3b",  "01:1e:67:d8:41:3c" ], "EndpointEnabled": true },
        { "Id" : "x1c3s28b0n0","NID":1504, "FQDN" : "x1c3s28b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:81:a2",  "01:1e:67:e3:81:a3" ], "EndpointEnabled": true },
        { "Id" : "x1c3s29b0n0","NID":1508, "FQDN" : "x1c3s29b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:3c:42",  "01:1e:67:e3:3c:43" ], "EndpointEnabled": true },
        { "Id" : "x1c3s30b0n0","NID":1512, "FQDN" : "x1c3s30b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:df:fc:3f",  "01:1e:67:df:fc:40" ], "EndpointEnabled": true },
        { "Id" : "x1c3s31b0n0","NID":1516, "FQDN" : "x1c3s31b0n0.test.com",
          "State":"Ready",
          "MAC":[  "01:1e:67:e3:46:d8",  "01:1e:67:e3:46:d9" ], "EndpointEnabled": true }
    ] }`
