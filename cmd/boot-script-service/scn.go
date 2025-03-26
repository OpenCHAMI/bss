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

// Shasta boot script server state change notification management
//
// Set up state change notification subscriptions in order to keep the known
// configuration up-to-date with the state manager.
package main

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"
)

const (
	defaultNFDHost     = "cray-hmnfd"
	defaultNFDBaseURI  = "/hmi/v1"
	defaultNFDScheme   = "http"
	UpdateTimestampKey = "/UpdateTimestamp" // etcd key for update timestamp
)

type ScnNotifier struct {
	SubscriberName string
	SubscriberURL  string
	NotifierURL    string
	Components     []string
	Client         *http.Client
}

type Scn struct {
	Components []string `json:"Components"`
	Enabled    *bool    `json:"Enabled,omitempty"`
	//Flag string           `json:"Flag,omitempty"`
	Role           string `json:"Role,omitempty"`
	SubRole        string `json:"SubRole,omitempty"`
	SoftwareStatus string `json:"SoftwareStatus,omitempty"`
	State          string `json:"State,omitempty"`
}

type ScnSubscribe struct {
	Subscriber     string   `json:"Subscriber"`
	Components     []string `json:"Components,omitempty"`
	Url            string   `json:"Url"`
	States         []string `json:"States,omitempty"`
	Enabled        *bool    `json:"Enabled,omitempty"`
	SoftwareStatus []string `json:"SoftwareStatus,omitempty"`
	Roles          []string `json:"Roles,omitempty"`
	SubRoles       []string `json:"SubRoles,omitempty"`
}

// This function is called when hmnfd POSTs something to our notification URL
func stateChangeNotification(w http.ResponseWriter, r *http.Request) {
	p, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("ERROR reading body of POST from hmnfd")
		return
	}
	var scn Scn
	if err = json.Unmarshal(p, &scn); err != nil {
		log.Printf("ERROR reading body of POST from hmnfd")
		w.WriteHeader(http.StatusBadRequest)
		// FIXME: Add error return data
		return
	}
	log.Printf("Received state change notification: %s", p)
	// We simply store a timestamp.  This is the approx. time that SM updated
	// something.  The next time BSS needs to check a host, it will see if it
	// is up-to-date, and if not, it will fetch new SM data at that time.
	// This has the advantage of not needing to fetch this data if BSS doesn't
	// need it.  Additional updates to SM can then be made without BSS
	// fetching the intermediate state.  The disadvantage is that it needs to
	// get everything all at once.  The time isn't all that critical since it
	// will respond to immediate requests with a chained response to have the
	// requester try again after a short delay, giving BSS time to retrieve
	// the SM data.
	timestamp := strconv.FormatInt(time.Now().Unix(), 10)
	if err = kvstore.Store(UpdateTimestampKey, timestamp); err != nil {
		log.Printf("Failed to store update timestamp %s to key %s: %s",
			timestamp, UpdateTimestampKey, err)
	}
}

// Checks the current timestamp of this running image vs. the timestamp in etcd.
// Will do a refresh if needed.
func checkState(force bool) bool {
	var (
		timestamp string
		exists    bool
		ts        int64
		err       error
	)
	if force {
		ts = -1
	} else {
		timestamp, exists, _ = kvstore.Get(UpdateTimestampKey)
		ts, err = strconv.ParseInt(timestamp, 0, 64)
	}
	if force || exists && err == nil && smTimeStamp < ts {
		debugf("force: %t, exists: %t, timestamp = %s, ts = %d, smTimeStamp = %d", force, exists, timestamp, ts, smTimeStamp)
		go refreshState(ts)
		return true
	}
	return false
}
