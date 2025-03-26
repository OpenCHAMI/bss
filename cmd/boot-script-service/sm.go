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
	"crypto/tls"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const (
	badMAC       = "not available"
	undefinedMAC = "ff:ff:ff:ff:ff:ff"
	hsmTestEP    = "/Inventory/RedfishEndpoints"
)

var (
	smMutex      sync.Mutex
	smData       *SMData
	smClient     *OAuthClient
	smDataMap    map[string]SMComponent
	smBaseURL    string
	smJSONFile   string
	smTimeStamp  int64
	stateManager StateManager
)

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
		err = fmt.Errorf("could not join URL paths %q and %q: %v", smBaseURL, hsmTestEP, err)
		return
	}

	retryDuration, err := time.ParseDuration(fmt.Sprintf("%ds", retryInterval))
	if err != nil {
		err = fmt.Errorf("invalid retry interval: %v", err)
		return
	}
	for retry := uint64(0); retry < retryCount; retry++ {
		log.Printf("Attempting connection to %s (attempt %d/%d)", testURL, retry+1, retryCount)
		resp, err = smClient.Get(testURL)
		if err != nil {
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

	err = fmt.Errorf("number of retries (%d) exhausted when testing if SMD auth is enabled", retryCount)
	return
}

func TestSMProtectedAccess() error {
	var (
		req *http.Request
		res *http.Response
	)

	if accessToken == "" {
		return fmt.Errorf("access token is empty")
	}
	if smClient == nil {
		return fmt.Errorf("smClient nil. Has a connection been opened yet?")
	}

	testURL, err := url.JoinPath(smBaseURL, hsmTestEP)
	if err != nil {
		err = fmt.Errorf("could not join URL paths %q and %q: %v", smBaseURL, hsmTestEP, err)
		return err
	}

	req, _ = http.NewRequest(http.MethodGet, testURL, nil)
	headers := map[string][]string{
		"Authorization": {"Bearer " + accessToken},
	}
	req.Header = headers
	res, err = smClient.Do(req)
	if err != nil {
		return fmt.Errorf("could not execute request: %v", err)
	}
	defer res.Body.Close()

	return nil
}

func SmOpen(base, options string) error {
	u, err := url.Parse(base)
	if err != nil {
		return fmt.Errorf("unknown HSM URL: %s", base)
	}
	if u.Scheme == "mem" {
		// The mem: interface to the state manager is strictly for testing
		// purposes.  A canned set of pre-defined nodes are loaded into memory
		// and used as state manager data.  This allows for testing of a larger
		// set of nodes than is currently readily available.
		debugf("Setting internal HSM data")
		stateManager = NewFileStateManager("")
		return nil
	}
	if u.Scheme == "file" {
		// The file: interface allows for another method of testing with a
		// little more flexibilty than the mem: interface, but not quite as
		// stand-alone.
		smJSONFile = u.Path
		debugf("Setting externel HSM data file: %s", smJSONFile)
		stateManager = NewFileStateManager(smJSONFile)
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
	smBaseURL = base
	log.Printf("Accessing state manager via %s\n", smBaseURL)

	var smAuthEnabled bool
	smAuthEnabled, err = TestSMAuthEnabled(authRetryCount, authRetryWait)
	if err != nil {
		return fmt.Errorf("failed testing if HSM auth is enabled: %v", err)
	}
	if smAuthEnabled {
		log.Printf("HSM authenticated endpoints enabled, checking token")
		err = smClient.JWTTestAndRefresh()
		if err != nil {
			return fmt.Errorf("failed refreshing JWT: %v", err)
		}
	}

	stateManager = NewHSMStateManager(smBaseURL, smClient)
	return nil
}

func getStateInfo() (ret *SMData) {
	if stateManager != nil {
		var err error
		ret, err = stateManager.GetState()
		if err != nil {
			log.Printf("Error getting state: %v", err)
		}
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
			smDataMap = make(map[string]SMComponent)
			for _, comp := range smData.Components {
				smDataMap[comp.ID] = comp
			}
		}
	}
	return smData, smDataMap
}

func getState() *SMData {
	data, _ := protectedGetState(0)
	return data
}

func refreshState(ts int64) *SMData {
	data, _ := protectedGetState(ts)
	return data
}

func FindSMCompByMAC(mac string) (SMComponent, bool) {
	if stateManager != nil {
		return stateManager.GetComponentByMAC(mac)
	}
	return SMComponent{}, false
}

func FindSMCompByNameInCache(host string) (SMComponent, bool) {
	if stateManager != nil {
		return stateManager.GetComponentByName(host)
	}
	return SMComponent{}, false
}

func FindSMCompByName(host string) (SMComponent, bool) {
	if stateManager != nil {
		return stateManager.GetComponentByName(host)
	}
	return SMComponent{}, false
}

func FindSMCompByNid(nid int) (SMComponent, bool) {
	if stateManager != nil {
		return stateManager.GetComponentByNID(nid)
	}
	return SMComponent{}, false
}

func FindXnameByIP(ip string) (string, bool) {
	state := getState()
	if state == nil {
		return "", false
	}
	if eth, ok := state.IPAddrs[ip]; ok {
		return eth.CompID, true
	}
	return "", false
}
