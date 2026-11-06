// SPDX-FileCopyrightText: Copyright © 2026 OpenCHAMI a Series of LF Projects, LLC
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"errors"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	hmetcd "github.com/Cray-HPE/hms-hmetcd"
	hms_s3 "github.com/Cray-HPE/hms-s3"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/openchami/bss/pkg/bssTypes"
	sm "github.com/openchami/smd/v2/pkg/sm"
)

type phase1FailingStore struct{ hmetcd.Kvi }

func (s *phase1FailingStore) Store(string, string) error {
	return errors.New("fixture-backend-error")
}

func phase1KnownNode(t *testing.T) {
	t.Helper()
	usePhase1Store(t)
	oldState, oldMap, oldStamp, oldSQL := smData, smDataMap, smTimeStamp, useSQL
	oldBlocked := blockedRoles
	t.Cleanup(func() {
		smData, smDataMap, smTimeStamp, useSQL = oldState, oldMap, oldStamp, oldSQL
		blockedRoles = oldBlocked
	})
	comp, ok := FindSMCompByName("x0c0s1b0n0")
	if !ok {
		t.Fatal("missing known-node fixture")
	}
	comp.EndpointEnabled = true
	smData = &SMData{Components: []SMComponent{comp}, IPAddrs: map[string]sm.CompEthInterfaceV2{"192.0.2.10": {CompID: comp.ID}}}
	smDataMap, smTimeStamp, useSQL, blockedRoles = makeSmMap(smData), time.Now().Unix(), false, nil
}

func TestPhase1PhoneHomeStorageFailureRedacted(t *testing.T) {
	phase1KnownNode(t)
	const secret = "fixture-existing-phone-home-secret"
	if err, _ := Store(bssTypes.BootParams{Hosts: []string{"x0c0s1b0n0"}, Params: "secret=" + secret, Kernel: "https://images.invalid/phone-home-kernel"}); err != nil {
		t.Fatal(err)
	}
	kvstore = &phase1FailingStore{Kvi: kvstore}
	oldOutput := log.Writer()
	t.Cleanup(func() { log.SetOutput(oldOutput) })
	var output bytes.Buffer
	log.SetOutput(&output)
	req := httptest.NewRequest(http.MethodPost, "/phone-home", strings.NewReader(`{"instance_id":"fixture-completion"}`))
	req.RemoteAddr = "192.0.2.10:12345"
	w := httptest.NewRecorder()
	phoneHomePost(w, req)
	if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), secret) {
		t.Fatal("phone-home storage-error response contract changed or failure was not exercised")
	}
	if !strings.Contains(output.String(), "/phone-home FAILED") || strings.Contains(output.String(), secret) {
		t.Error("phone-home storage error logged boot credentials or lost its operation event")
	}
}

func TestPhase1BootscriptSignerFailureRedacted(t *testing.T) {
	phase1KnownNode(t)
	const sensitiveError = "https://signer.invalid/?credential=fixture-signer-secret"
	if err, _ := Store(bssTypes.BootParams{Hosts: []string{"x0c0s1b0n0"}, Kernel: "s3://fixture/kernel"}); err != nil {
		t.Fatal(err)
	}
	client, err := hms_s3.NewS3Client(hms_s3.ConnectionInfo{AccessKey: "fixture-access", SecretKey: "fixture-secret", Endpoint: "https://s3.invalid", Region: "fixture", Bucket: "fixture"}, &http.Client{})
	if err != nil {
		t.Fatal(err)
	}
	client.S3.Handlers.Sign.Clear()
	client.S3.Handlers.Sign.PushBack(func(req *request.Request) { req.Error = errors.New(sensitiveError) })
	oldClient, oldDebug, oldOutput := s3Client, debugFlag, log.Writer()
	t.Cleanup(func() { s3Client, debugFlag = oldClient, oldDebug; log.SetOutput(oldOutput) })
	s3Client, debugFlag = client, true
	var output bytes.Buffer
	log.SetOutput(&output)
	w := httptest.NewRecorder()
	BootscriptGet(w, httptest.NewRequest(http.MethodGet, "/boot/v1/bootscript?name=x0c0s1b0n0", nil))
	if w.Code != http.StatusNotFound || !strings.Contains(w.Body.String(), "fixture-signer-secret") {
		t.Fatal("bootscript signer-error response contract changed or failure was not exercised")
	}
	if !strings.Contains(output.String(), "BSS request failed") || strings.Contains(output.String(), "fixture-signer-secret") {
		t.Error("bootscript failure logged signer credentials or lost its operation event")
	}
}
