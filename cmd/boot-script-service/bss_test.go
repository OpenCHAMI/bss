// MIT License
//
// (C) Copyright [2021-2022] Hewlett Packard Enterprise Development LP
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
// Boot script Service Unit tests.
//

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/OpenCHAMI/bss/pkg/bssTypes"
)

func TestMain(m *testing.M) {
	// call flag.Parse() here if TestMain uses flags

	var err error

	// Now we make a Kv object to mock up the KV store service.
	err = kvOpen("mem:", "", 1, 1)
	excode := 1
	if err != nil {
		// This should not happen as long as the SM json is correct.
		fmt.Fprintf(os.Stderr, "Test SM data decode failed: %v\n", err)
	} else {
		SmOpen("mem:", "")
		excode = m.Run()
	}
	os.Exit(excode)
}

func TestFindSM(t *testing.T) {
	tables := []struct {
		host string
		nid  int64
		mac1 string
		mac2 string
	}{
		{"x0c0s1b0n0", 8, "00:1e:67:e3:46:51", "00:1e:67:e3:46:52"},
		{"x0c0s5b0n0", 24, "00:1e:67:d8:9a:e1", "00:1e:67:d8:9a:e2"},
		{"x0c0s18b0n0", 76, "00:1e:67:dd:d1:7c", "00:1e:67:dd:d1:7d"},
		{"x0c1s21b0n0", 216, "00:1e:67:d6:24:ce", "00:1e:67:d6:24:cf"},
		{"x0c3s5b0n0", 412, "00:1e:67:dd:d0:eb", "00:1e:67:dd:d0:ec"},
	}

	for _, tbl := range tables {
		c, ok := FindSMCompByName(tbl.host)
		if !ok {
			t.Errorf("FindSMCompByName failed, '%s' not found\n", tbl.host)
		} else if c.ID != tbl.host {
			t.Errorf("FindSMCompByName miscompare, expected '%s', got '%s'\n", tbl.host, c.ID)
		} else if nid, err := c.NID.Int64(); err != nil || nid != tbl.nid {
			t.Errorf("FindSMCompByName miscompare, expected Nid %d, got %d\n", tbl.nid, nid)
		}
	}
	for _, tbl := range tables {
		c, ok := FindSMCompByNid(int(tbl.nid))
		if !ok {
			t.Errorf("FindSMCompByNid failed, '%s' not found\n", tbl.host)
		} else if c.ID != tbl.host {
			t.Errorf("FindSMCompByNid miscompare, expected '%s', got '%s'\n", tbl.host, c.ID)
		} else if nid, err := c.NID.Int64(); err != nil || nid != tbl.nid {
			t.Errorf("FindSMCompByNid miscompare, expected Nid %d, got %d\n", tbl.nid, nid)
		}
	}
	for _, tbl := range tables {
		c, ok := FindSMCompByMAC(tbl.mac1)
		if !ok {
			t.Errorf("FindSMCompByMAC failed, '%s' not found\n", tbl.host)
		} else if c.ID != tbl.host {
			t.Errorf("FindSMCompByMAC miscompare, expected '%s', got '%s'\n", tbl.host, c.ID)
		} else if nid, err := c.NID.Int64(); err != nil || nid != tbl.nid {
			t.Errorf("FindSMCompByMAC miscompare, expected Nid %d, got %d\n", tbl.nid, nid)
		}
	}
	for _, tbl := range tables {
		c, ok := FindSMCompByMAC(tbl.mac2)
		if !ok {
			t.Errorf("FindSMCompByMAC failed, '%s' not found\n", tbl.host)
		} else if c.ID != tbl.host {
			t.Errorf("FindSMCompByMAC miscompare, expected '%s', got '%s'\n", tbl.host, c.ID)
		} else if nid, err := c.NID.Int64(); err != nil || nid != tbl.nid {
			t.Errorf("FindSMCompByMAC miscompare, expected Nid %d, got %d\n", tbl.nid, nid)
		}
	}
}

func TestStoreAndLookup(t *testing.T) {
	const vmlinuz string = "/test/path/vmlinuz"
	tables := []bssTypes.BootParams{
		{Hosts: []string{"Default"}, Params: "default", Kernel: vmlinuz, Initrd: "/test/path/initrd.gz"},
		{Hosts: []string{"x0c0s1b0n0"}, Params: "test-c0s1", Kernel: "/test/path/vmlinuz", Initrd: "/test/path/initrd.gz"},
		{Hosts: []string{"x0c2s13b0n0"}, Params: "test-c2s13", Kernel: "/test/path/vmlinuz", Initrd: "/test/path/initrd.gz"},
		{Kernel: "/test/path/vmlinuz", Params: "def-vmlinuz"},
		{Initrd: "/test/path/initrd.gz", Params: "def-initrd"},
	}
	for _, bp := range tables {
		err, referralToken := Store(bp)
		if err != nil {
			t.Errorf("Store failed for '%v': %s", bp, err.Error())
		} else if referralToken == "" && (bp.Hosts != nil || bp.Nids != nil || bp.Macs != nil) {
			t.Errorf("Store failed to create a referral token for '%v'", bp)
		} else if referralToken != "" && (bp.Hosts == nil && bp.Nids == nil && bp.Macs == nil) {
			t.Errorf("Store incorrectly created a referral token when only setting the kernel or initrd values for '%v'", bp)
		}
	}

	test_tables := []struct {
		host  string
		param string
	}{
		{"x0c0s1b0n0", "test-c0s1"},
		{"x0c0s5b0n0", "default"},
		{"x0c2s13b0n0", "test-c2s13"},
	}

	for _, tt := range test_tables {
		bd, sc := LookupByName(tt.host)
		if bd.Kernel.Path != vmlinuz {
			t.Errorf("LookupByName(\"%s\") failed: kernel path expected: %s, actual: %s\n",
				tt.host, vmlinuz, bd.Kernel.Path)
		}
		if sc.ID != tt.host {
			t.Errorf("LookupByName(\"%s\") failed: Id expected: %s, actual: %s\n",
				tt.host, tt.host, sc.ID)
		}
		if bd.Params != tt.param {
			t.Errorf("LookupByName(\"%s\") failed: Params expected: %s, actual: %s\n",
				tt.host, tt.param, bd.Params)
		}
	}
	/***************************************************************************
	 * Now test a http handler.
	 **************************************************************************/
	// Create a request to pass to our handler. We don't have any query
	// parameters for now, so we'll pass 'nil' as the third parameter.
	//body := bytes.NewBufferString(`{"hosts":["x0c0s1n0"]}`)
	body := bytes.NewBufferString("")
	req, err := http.NewRequest("GET", "/boot/v1/bootparameter", body)
	if err != nil {
		t.Fatal(err)
	}

	// We create a ResponseRecorder (which satisfies http.ResponseWriter) to
	// record the response.
	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(BootparametersGet)

	// Our handlers satisfy http.Handler, so we can call their ServeHTTP method
	// directly and pass in our Request and ResponseRecorder.
	handler.ServeHTTP(rr, req)

	// Check the status code is what we expect.
	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusOK)
	}

	// Check the response body is what we expect.
	fmt.Printf("BootparametersGet() returns %v\n", rr.Body.String())
	var bplist []bssTypes.BootParams
	buf := bytes.NewBufferString(rr.Body.String())
	dec := json.NewDecoder(buf)
	err = dec.Decode(&bplist)
	if err != nil {
		// This should not happen as long as the SM json is correct.
		t.Errorf("BootParam returned data decode failed: %v\n", err)
	} else {
		fmt.Printf("Got response: %v\n", bplist)
	}
	if len(bplist) != len(tables) {
		t.Errorf("Incorrect number of responses returned, expected %d, got %d\n",
			len(tables), len(bplist))
	}
}
