// SPDX-FileCopyrightText: Copyright © 2026 OpenCHAMI a Series of LF Projects, LLC
// SPDX-License-Identifier: MIT

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/OpenCHAMI/jwtauth/v5"
	"github.com/openchami/bss/pkg/bssTypes"
)

type phase1TokenTransport func(*http.Request) (*http.Response, error)

func (transport phase1TokenTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return transport(request)
}

func TestPhase1RenderedCredentialsStayOutOfLogs(t *testing.T) {
	for _, debug := range []bool{false, true} {
		name := "normal"
		if debug {
			name = "debug"
		}
		t.Run(name, func(t *testing.T) {
			const joinToken = "fixture-generated-join-token"
			const referralToken = "fixture-referral-token"
			const signedURL = "https://images.invalid/kernel?signature=fixture-signed-url"
			oldClient, oldBase, oldRegex, oldDebug := spireTokenClient, spireTokensBaseURL, s3ParamsRegex, debugFlag
			oldOutput := log.Writer()
			t.Cleanup(func() {
				spireTokenClient, spireTokensBaseURL, s3ParamsRegex, debugFlag = oldClient, oldBase, oldRegex, oldDebug
				log.SetOutput(oldOutput)
			})
			var output bytes.Buffer
			log.SetOutput(&output)
			debugFlag = debug
			// Fail after join-token substitution, without changing signer behavior.
			s3ParamsRegex = "fixture-private-regex["
			spireTokensBaseURL = "http://spire.invalid"
			called := false
			spireTokenClient = &http.Client{Transport: phase1TokenTransport(func(*http.Request) (*http.Response, error) {
				called = true
				return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(`{"join_token":"` + joinToken + `"}`))}, nil
			})}
			script, err := buildBootScript(BootData{Params: "join=${SPIRE_JOIN_TOKEN} secret=fixture-boot-secret", Kernel: ImageData{Path: signedURL}}, scriptParams{xname: "x0c0s2b0n0", referralToken: referralToken}, "https://boot.invalid/retry", "Compute", "", "test node")
			if err != nil {
				t.Fatal(err)
			}
			if !called || !strings.Contains(script, joinToken) || !strings.Contains(script, referralToken) || !strings.Contains(script, signedURL) {
				t.Fatal("rendered boot response lost required credentials")
			}
			if !strings.Contains(output.String(), "event=s3_parameter_replacement_failed") {
				t.Error("missing sanitized failure diagnostic")
			}
			for _, secret := range []string{joinToken, referralToken, signedURL, "fixture-boot-secret", s3ParamsRegex} {
				if strings.Contains(output.String(), secret) {
					t.Error("rendering logged credential-bearing data")
				}
			}
		})
	}
}

func TestPhase1StoredCredentialsStayOutOfLogs(t *testing.T) {
	usePhase1Store(t)
	oldDebug, oldSQL, oldOutput := debugFlag, useSQL, log.Writer()
	t.Cleanup(func() { debugFlag, useSQL = oldDebug, oldSQL; log.SetOutput(oldOutput) })
	debugFlag, useSQL = true, false
	var output bytes.Buffer
	log.SetOutput(&output)
	bp := bssTypes.BootParams{Hosts: []string{"x0c0s2b0n0"}, Params: "secret=fixture-stored-secret", Kernel: "https://images.invalid/kernel?signature=fixture-stored-url"}
	err, token := Store(bp)
	if err != nil || token == "" {
		t.Fatal("could not store fixture boot parameters")
	}
	bd, _ := LookupByName(bp.Hosts[0])
	if bd.Params != bp.Params || bd.Kernel.Path != bp.Kernel || bd.ReferralToken != token {
		t.Fatal("stored boot parameters or referral token changed")
	}
	LogBootParameters("/bootparameters POST", bp)
	if !strings.Contains(output.String(), "/bootparameters POST") {
		t.Error("missing operation diagnostic")
	}
	for _, secret := range []string{bp.Params, bp.Kernel, token} {
		if strings.Contains(output.String(), secret) {
			t.Error("storage logged credential-bearing data")
		}
	}
}

func TestPhase1SafeBSSReadsRemainAnonymous(t *testing.T) {
	usePhase1Store(t)
	oldURL, oldAuth, oldSQL := jwksURL, tokenAuth, useSQL
	oldDebug, oldOutput := debugFlag, log.Writer()
	t.Cleanup(func() {
		jwksURL, tokenAuth, useSQL = oldURL, oldAuth, oldSQL
		debugFlag = oldDebug
		log.SetOutput(oldOutput)
	})
	useSQL = false
	debugFlag = true
	var output bytes.Buffer
	log.SetOutput(&output)
	const params = "secret=fixture-public-boot-secret"
	if err, _ := Store(bssTypes.BootParams{Hosts: []string{"x0c0s2b0n0"}, Params: params, Kernel: "https://images.invalid/phase1-public-kernel"}); err != nil {
		t.Fatal(err)
	}
	for _, configuredAuth := range []bool{false, true} {
		jwksURL = ""
		if configuredAuth {
			jwksURL = "https://keys.invalid/jwks"
		}
		tokenAuth = jwtauth.New("HS256", []byte("fixture-signing-key"), nil)
		router := initHandlers()
		for _, path := range []string{"/boot/v1/dumpstate", "/boot/v1/endpoint-history", "/boot/v1/hosts"} {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
			if w.Code != http.StatusOK || !json.Valid(w.Body.Bytes()) {
				t.Errorf("anonymous GET %s with configuredAuth=%t: status%d, validJSON=%t", path, configuredAuth, w.Code, json.Valid(w.Body.Bytes()))
			}
			if path == "/boot/v1/dumpstate" && !strings.Contains(w.Body.String(), params) {
				t.Error("dumpstate no longer returns existing boot parameters")
			}
		}
		if configuredAuth {
			w := httptest.NewRecorder()
			router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/boot/v1/bootparameters", nil))
			if w.Code != http.StatusUnauthorized {
				t.Errorf("existing bootparameters authentication changed: got%d", w.Code)
			}
		}
	}
	if strings.Contains(output.String(), params) {
		t.Error("public read copied boot credentials into diagnostics")
	}
}

func TestPhase1CloudDataMergeFailureRedacted(t *testing.T) {
	oldOutput := log.Writer()
	t.Cleanup(func() { log.SetOutput(oldOutput) })
	var output bytes.Buffer
	log.SetOutput(&output)
	existing := bssTypes.CloudDataType{"secret": "fixture-existing-cloud-secret"}
	merge := bssTypes.CloudDataType{"secret": "fixture-new-cloud-secret", "unsupported": make(chan int)}
	if updateCloudData(&existing, merge, "meta-data") {
		t.Fatal("failed merge changed its reported outcome")
	}
	if existing["secret"] != "fixture-existing-cloud-secret" {
		t.Fatal("failed merge modified existing data")
	}
	if !strings.Contains(output.String(), "PATCH request for meta-data failed") {
		t.Error("missing cloud-data failure diagnostic")
	}
	if strings.Contains(output.String(), "fixture-existing-cloud-secret") || strings.Contains(output.String(), "fixture-new-cloud-secret") {
		t.Error("failed cloud-data merge logged its payload")
	}
}
