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
// Cloud initialization API functionality

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"strings"

	"github.com/OpenCHAMI/bss/pkg/bssTypes"

	yaml "gopkg.in/yaml.v2"

	base "github.com/Cray-HPE/hms-base"
)

const QUERYKEY = "key"

func mapLookup(m map[string]interface{}, keys ...string) (interface{}, error) {
	var ok bool
	var foundObj interface{}

	if len(keys) == 0 {
		return nil, fmt.Errorf("mapLookup needs at least one key")
	}
	if foundObj, ok = m[keys[0]]; !ok {
		return nil, fmt.Errorf("key not found in map; keys: %v", keys)
	} else if len(keys) == 1 {
		return foundObj, nil
	} else if m, ok = foundObj.(map[string]interface{}); !ok {
		return nil, fmt.Errorf("malformed structure at %#v", foundObj)
	} else {
		return mapLookup(m, keys[1:]...)
	}
}

func generateInstanceID(prefix string) string {
	if prefix == "" {
		prefix = "default"
	}
	b := make([]byte, 6)
	rand.Read(b)
	return strings.ToLower(fmt.Sprintf("%s-%X", prefix, b))
}

func findRemoteAddr(r *http.Request) string {
	remoteaddr := r.Header.Get("X-Forwarded-For")
	if remoteaddr == "" {
		// Since IPV6 address have colons we only strip the last colon which
		// is the port. We know this from the http docs indicating IP:PORT
		// will always be set. https://golang.org/pkg/net/http/#Request
		remoteaddrSlice := strings.Split(r.RemoteAddr, ":")
		remoteaddr = strings.Join(remoteaddrSlice[:len(remoteaddrSlice)-1], ":")
	} else {
		// XFF is a comma seperated list of IPs forwarded through.
		// Envoy will append the trusted client IP, which is what we want.
		remoteaddrSlice := strings.Split(remoteaddr, ",")
		remoteaddr = remoteaddrSlice[len(remoteaddrSlice)-1]
	}
	return remoteaddr
}

// generateMetaData attempts to inject and discoverable meta-data we know about
// from HSM or elsewhere.
func generateMetaData(xname string, metadata map[string]interface{}) error {
	// TODO: Attempt to get the hostname, region, and az from SLS aliases

	metadata["instance-id"] = generateInstanceID(xname)

	comp, found := FindSMCompByName(xname)
	if !found {
		return fmt.Errorf("Could not find Component for %s", xname)
	}

	if metadata["local-hostname"] == nil {
		metadata["local-hostname"] = xname
	}

	if metadata["shasta-type"] == nil {
		metadata["shasta-type"] = comp.Role
	}

	if metadata["shasta-role"] == nil {
		metadata["shasta-role"] = comp.SubRole
	}

	return nil
}

// Merge values from second into first. We will only handle nested maps,
// slices will always favor second over first.
func mergeMaps(first, second map[string]interface{}) map[string]interface{} {
	for key, secondVal := range second {
		if firstVal, present := first[key]; present {
			switch firstVal.(type) {
			case map[string]interface{}:
				// value is also a map interface, so recurse into it
				first[key] = mergeMaps(firstVal.(map[string]interface{}), secondVal.(map[string]interface{}))
				continue
			default:
				first[key] = secondVal
			}
		} else {
			// key not in first so add it
			first[key] = secondVal
		}
	}
	return first
}

func metaDataGetAPI(w http.ResponseWriter, r *http.Request) {
	var respData map[string]interface{}
	var httpStatus = http.StatusOK
	var isDefault = false

	remoteaddr := findRemoteAddr(r)

	// Get the xname to lookup metadata.
	xname, found := FindXnameByIP(remoteaddr)
	if !found {
		isDefault = true
		log.Printf("CloudInit -> No XName found for: %s, using default data\n", remoteaddr)
	}

	// If name is "" here, LookupByName uses the default tag, which is what we want.
	bootdata, _ := LookupByName(xname)
	globaldata, _ := LookupGlobalData()

	log.Printf("GET /meta-data, xname: %s ip: %s", xname, remoteaddr)
	respData = bootdata.CloudInit.MetaData
	// If empty, initialize an empty map
	if len(respData) == 0 {
		respData = make(map[string]interface{})
	}

	if isDefault {
		respData["instance-id"] = generateInstanceID("")
	} else {
		err := generateMetaData(xname, respData)
		if err != nil {
			log.Printf("Warning - %s: Some meta data could not be found!\n", xname)
		}
	}

	roleData := BootData{}
	shastaRole := respData["shasta-role"]
	if shastaRole != nil {
		roleData, _ = LookupByRole(shastaRole.(string))
	}
	roleInitData := roleData.CloudInit.MetaData
	if len(roleInitData) == 0 {
		roleInitData = make(map[string]interface{})
	}

	// Override any role data from the per node data
	mergedData := mergeMaps(roleInitData, respData)

	globalRespData := globaldata.CloudInit.MetaData
	// If empty, initialize an empty map
	if len(globalRespData) == 0 {
		globalRespData = make(map[string]interface{})
	}

	mergedData["Global"] = globalRespData
	queries := r.URL.Query()

	lookupKeys, ok := queries[QUERYKEY]
	if ok && len(lookupKeys) > 0 {
		// Query string provided in request, return it.
		lookupKey := strings.Split(lookupKeys[0], ".")
		rval, err := mapLookup(mergedData, lookupKey...)
		if err != nil {
			debugf("CloudInit MetaData: Query Not Found: %v\n", err)
			base.SendProblemDetailsGeneric(w, http.StatusNotFound,
				fmt.Sprintf("Not Found"))
			return
		}
		json.NewEncoder(w).Encode(rval)
	} else {
		// No query, return all data
		json.NewEncoder(w).Encode(mergedData)
	}

	w.WriteHeader(httpStatus)
	return

}

func userDataGetAPI(w http.ResponseWriter, r *http.Request) {
	var respData map[string]interface{}
	var httpStatus = http.StatusOK
	isDefault := false

	remoteaddr := findRemoteAddr(r)

	// Get the xname to lookup metadata.
	xname, found := FindXnameByIP(remoteaddr)
	if !found {
		isDefault = true
		log.Printf("CloudInit -> No XName found for: %s, using default data\n", remoteaddr)
	}

	// If name is "" here, LookupByName uses the default tag, which is what we want.
	bootdata, _ := LookupByName(xname)
	metaData := bootdata.CloudInit.MetaData

	if len(metaData) == 0 {
		metaData = make(map[string]interface{})
	}

	if !isDefault {
		err := generateMetaData(xname, metaData)
		if err != nil {
			log.Printf("Warning - %s: Some meta data could not be found!\n", xname)
		}
	}

	roleData := BootData{}
	shastaRole := metaData["shasta-role"]
	if shastaRole != nil {
		roleData, _ = LookupByRole(shastaRole.(string))
	}
	roleInitData := roleData.CloudInit.UserData
	if len(roleInitData) == 0 {
		roleInitData = make(map[string]interface{})
	}

	log.Printf("GET /user-data, xname: %s ip: %s", xname, remoteaddr)
	respData = bootdata.CloudInit.UserData
	if len(respData) == 0 {
		respData = make(map[string]interface{})
	}

	// Override any role data from the per node data
	mergedData := mergeMaps(roleInitData, respData)

	if mergedData["local-hostname"] == nil && metaData["local-hostname"] != nil {
		mergedData["local-hostname"] = metaData["local-hostname"]
	}

	databytes, err := yaml.Marshal(mergedData)
	if err != nil {
		base.SendProblemDetailsGeneric(w, http.StatusBadRequest, "Invalid YAML")
		return
	}

	w.Header().Set("Content-Type", "text/yaml")
	w.WriteHeader(httpStatus)
	_, _ = fmt.Fprintf(w, "#cloud-config\n%s", string(databytes))

	// Record the fact this was asked for.
	updateEndpointAccessed(xname, bssTypes.EndpointTypeUserData)

	return
}

func endpointHistoryGetAPI(w http.ResponseWriter, r *http.Request) {
	debugf("endpointHistoryGetAPI(): Received request %v\n", r.URL)

	r.ParseForm() // r.Form is empty until after parsing
	name := strings.Join(r.Form["name"], "")
	endpoint := strings.Join(r.Form["endpoint"], "")

	var lastAccessTypeStruct bssTypes.EndpointType

	if endpoint != "" {
		lastAccessTypeStruct = bssTypes.EndpointType(endpoint)
	}

	var (
		accesses []bssTypes.EndpointAccess
		err      error
	)
	if useSQL {
		accesses, err = bssdb.SearchEndpointAccesses(name, bssTypes.EndpointType(endpoint))
	} else {
		accesses, err = SearchEndpointAccessed(name, lastAccessTypeStruct)
	}
	if err != nil {
		errMsg := fmt.Sprintf("Failed to search for name=%q, endpoint=%q: %v", name, endpoint, err)
		base.SendProblemDetailsGeneric(w, http.StatusInternalServerError, errMsg)
		log.Printf("BSS request failed: %s", errMsg)
		return
	}

	if len(accesses) == 0 {
		// Always make sure to give back at least an empty array instead of `null`.
		accesses = []bssTypes.EndpointAccess{}
	}

	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	err = json.NewEncoder(w).Encode(accesses)
	if err != nil {
		log.Printf("Yikes, I couldn't encode a JSON status response: %s\n", err)
	}
}

func phoneHomePostAPI(w http.ResponseWriter, r *http.Request) {
	var bp bssTypes.BootParams
	var hosts []string

	var args bssTypes.PhoneHome
	dec := json.NewDecoder(r.Body)
	err := dec.Decode(&args)
	if err != nil {
		debugf("CloudInit PhoneHome: Bad Request: %v\n", err)
		base.SendProblemDetailsGeneric(w, http.StatusBadRequest,
			fmt.Sprintf("Bad Request"))
		return
	}

	remoteaddr := findRemoteAddr(r)
	// Get the xname to lookup metadata.
	xname, found := FindXnameByIP(remoteaddr)
	if !found {
		debugf("CloudInit -> Phone Home called for unknown xname, ip: %s", remoteaddr)
		base.SendProblemDetailsGeneric(w, http.StatusNotFound,
			fmt.Sprintf("XName not found for IP"))
		return
	}
	hosts = append(hosts, xname)
	bootdata, _ := LookupByName(xname)

	bootdata.CloudInit.PhoneHome = args
	bp.Hosts = hosts
	bp.CloudInit = bootdata.CloudInit

	if err = Update(bp); err != nil {
		LogBootParameters(fmt.Sprintf("/phone-home FAILED: %s", err.Error()), args)
		base.SendProblemDetailsGeneric(w, http.StatusNotFound,
			fmt.Sprintf("Not Found: %s", err))
		return
	}

	log.Printf("POST /phone-home, xname: %s ip: %s", xname, remoteaddr)
	w.Header().Set("Content-Type", "application/json; charset=UTF-8")
	w.WriteHeader(http.StatusOK)
	out, _ := json.Marshal(bp)
	fmt.Fprintln(w, string(out))
}
