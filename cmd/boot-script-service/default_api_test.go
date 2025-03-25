// MIT License
//
// (C) Copyright [2022] Hewlett Packard Enterprise Development LP
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

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"

	"github.com/OpenCHAMI/bss/pkg/bssTypes"
)

func mockGetSignedS3Url(s3Url string) (string, error) {
	return s3Url + "_signed", nil
}

func mockGetSignedS3UrlError(s3Url string) (string, error) {
	return s3Url, fmt.Errorf("error")
}

func TestReplaceS3Params_regex(t *testing.T) {
	r, err := regexp.Compile(s3ParamsRegex)
	if err != nil {
		t.Errorf("Failed to compile the regex: %s, error: %v\n", s3ParamsRegex, err)
		return
	}
	params := fmt.Sprintf("%s%s",
		"metal.server=s3://b1/p1/p2",
		" metal.server=s3://b2/p1/p2")

	expected := [][]string{
		{
			"metal.server=s3://b1/p1/p2",
			"",
			"metal.server=s3://b1/p1/p2",
			"metal.server=",
			"s3://b1/p1/p2",
		},
		{
			" metal.server=s3://b2/p1/p2",
			" ",
			"metal.server=s3://b2/p1/p2",
			"metal.server=",
			"s3://b2/p1/p2",
		},
	}

	matches := r.FindAllStringSubmatch(params, -1)
	if len(matches) != 2 {
		t.Errorf("Failed expected two matches for: %s, using: %s\n", params, s3ParamsRegex)
		return
	}

	for i, match := range matches {
		if len(match) != 5 {
			t.Errorf("Failed. Expected %d match to have 5. groups: %v, params: %s\n", i, match, params)
			return
		}
		for j, group := range match {
			if group != expected[i][j] {
				t.Errorf("Failed wrong string for match %d group %d. expected: '%s', actual: '%s'\n",
					i, j, expected[i][j], group)
			}
		}
	}
}

// TestReplaceS3Params_replace_kernel_metal tests that the "metal.server=s3://<url>" argument is recognized and given a pre-signed URL.
func TestReplaceS3Params_replace_kernel_metal(t *testing.T) {
	params := fmt.Sprintf("%s %s %s %s %s",
		"metal.server=s3://ncn-images/k8s/0.2.78/filesystem.squashfs",
		"bond=bond0",
		"metal.server=s3://bucket/path",
		"root=craycps-s3:s3://boot-images",
		"m=s3://b/p")

	expected_params := fmt.Sprintf("%s %s %s %s %s",
		"metal.server=s3://ncn-images/k8s/0.2.78/filesystem.squashfs_signed",
		"bond=bond0",
		"metal.server=s3://bucket/path_signed",
		"root=craycps-s3:s3://boot-images",
		"m=s3://b/p")

	newParams, err := replaceS3Params(params, mockGetSignedS3Url)
	if err != nil {
		t.Errorf("replaceS3Params returned an error for params: %s, error: %v\n", params, err)
	}
	if newParams != expected_params {
		t.Errorf("replaceS3Params failed.\n  expected: %s\n  actual: %s\n", expected_params, newParams)
	}
}

// TestReplaceS3Params_replace_kernel_live tests that the dmsquash-live "root=live:s3://<url>" argument is recognized and given a pre-signed URL.
func TestReplaceS3Params_replace_kernel_live(t *testing.T) {
	params := fmt.Sprintf("%s %s %s %s",
		"root=live:s3://boot-images/k8s/0.2.78/rootfs",
		"bond=bond0",
		"root=live:s3://bucket/path",
		"m=s3://b/p")

	expected_params := fmt.Sprintf("%s %s %s %s",
		"root=live:s3://boot-images/k8s/0.2.78/rootfs_signed",
		"bond=bond0",
		"root=live:s3://bucket/path_signed",
		"m=s3://b/p")

	newParams, err := replaceS3Params(params, mockGetSignedS3Url)
	if err != nil {
		t.Errorf("replaceS3Params returned an error for params: %s, error: %v\n", params, err)
	}
	if newParams != expected_params {
		t.Errorf("replaceS3Params failed.\n  expected: %s\n  actual: %s\n", expected_params, newParams)
	}
}

func TestReplaceS3Params_replace2(t *testing.T) {
	params := fmt.Sprintf("%s %s",
		"xmetal.server=s3://ncn-images/k8s/0.2.78/filesystem.squashfs",
		"metal.server=s3://ncn-images/k8s/0.2.78/filesystem.squashfs")
	expected_params := fmt.Sprintf("%s %s",
		"xmetal.server=s3://ncn-images/k8s/0.2.78/filesystem.squashfs",
		"metal.server=s3://ncn-images/k8s/0.2.78/filesystem.squashfs_signed")

	newParams, err := replaceS3Params(params, mockGetSignedS3Url)
	if err != nil {
		t.Errorf("replaceS3Params returned an error for params: %s, error: %v\n", params, err)
	}
	if newParams != expected_params {
		t.Errorf("replaceS3Params failed.\n  expected: %s\n  actual: %s\n", expected_params, newParams)
	}
}

func TestReplaceS3Params_no_replace(t *testing.T) {
	// This test expects the string to remain unchanged
	params := fmt.Sprintf(
		"%s %s %s %s %s",
		"made_up_key=s3://ncn-images/path",
		"xmetal.server=s3://ncn-images/k8s/0.2.78/filesystem.squashfs",
		"nmd_data=url=s3://boot-images/bb-86/rootfs,etag=c8-204",
		"bos_update_frequency=4h",
		"root=craycps-s3:s3://boot-images/bb-78/rootfs:c8-204:dvs:api-gw-service-nmn.local:300:hsn0,nmn0:0")
	expected_params := params

	newParams, err := replaceS3Params(params, mockGetSignedS3Url)
	if err != nil {
		t.Errorf("replaceS3Params returned an error for params: %s, error: %v\n", params, err)
	}
	if newParams != expected_params {
		t.Errorf("replaceS3Params failed.\n  expected: %s\n  actual: %s\n", expected_params, newParams)
	}
}

func TestReplaceS3Params_error(t *testing.T) {
	params := "bond=bond0 metal.server=s3://bucket/path"
	expected_params := params
	newParams, err := replaceS3Params(params, mockGetSignedS3UrlError)
	if err == nil {
		t.Errorf("replaceS3Params failed to return an error when using mock that injects an error. params: %s\n", params)
	}
	if newParams != expected_params {
		t.Errorf("replaceS3Params failed.\n  expected: %s\n  actual: %s\n", expected_params, newParams)
	}
}

func TestBootparametersPost_Success(t *testing.T) {

	args := bssTypes.BootParams{
		Macs:   []string{"00:11:22:33:44:55"},
		Hosts:  []string{"x1c1s1b1n1"},
		Nids:   []int32{1},
		Kernel: "kernel",
		Initrd: "initrd",
		Params: "params",
	}

	body, _ := json.Marshal(args)
	req, err := http.NewRequest("POST", "/bootparameters", bytes.NewBuffer(body))
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(BootparametersPost)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusCreated {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusCreated)
	}

}

func TestBootparametersPost_BadRequest(t *testing.T) {

	req, err := http.NewRequest("POST", "/bootparameters", bytes.NewBuffer([]byte("invalid body")))
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(BootparametersPost)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusBadRequest)
	}
}

func TestBootparametersPost_InvalidMac(t *testing.T) {

	args := bssTypes.BootParams{
		Macs:   []string{"invalid-mac"},
		Hosts:  []string{"x1c1s1b1n1"},
		Nids:   []int32{1},
		Kernel: "kernel",
		Initrd: "initrd",
		Params: "params",
	}

	body, _ := json.Marshal(args)
	req, err := http.NewRequest("POST", "/bootparameters", bytes.NewBuffer(body))
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(BootparametersPost)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusBadRequest)
	}
}

func TestBootparametersPost_InvalidXname(t *testing.T) {

	args := bssTypes.BootParams{
		Macs:   []string{"00:11:22:33:44:55"},
		Hosts:  []string{"invalid-xname"},
		Nids:   []int32{1},
		Kernel: "kernel",
		Initrd: "initrd",
		Params: "params",
	}

	body, _ := json.Marshal(args)
	req, err := http.NewRequest("POST", "/bootparameters", bytes.NewBuffer(body))
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(BootparametersPost)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusBadRequest)
	}
}

func TestBootparametersPost_StoreError(t *testing.T) {

	args := bssTypes.BootParams{
		Macs:   []string{"00:11:22:33:44:55"},
		Hosts:  []string{"xname1"},
		Nids:   []int32{1},
		Kernel: "kernel",
		Initrd: "initrd",
		Params: "params",
	}

	body, _ := json.Marshal(args)
	req, err := http.NewRequest("POST", "/bootparameters", bytes.NewBuffer(body))
	if err != nil {
		t.Fatal(err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(BootparametersPost)
	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusBadRequest {
		t.Errorf("handler returned wrong status code: got %v want %v",
			status, http.StatusBadRequest)
	}
}
