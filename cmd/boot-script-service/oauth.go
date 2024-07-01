// Copyright © 2024 Triad National Security, LLC. All rights reserved.
//
// This program was produced under U.S. Government contract 89233218CNA000001
// for Los Alamos National Laboratory (LANL), which is operated by Triad
// National Security, LLC for the U.S. Department of Energy/National Nuclear
// Security Administration. All rights in the program are reserved by Triad
// National Security, LLC, and the U.S. Department of Energy/National Nuclear
// Security Administration. The Government is granted for itself and others
// acting on its behalf a nonexclusive, paid-up, irrevocable worldwide license
// in this material to reproduce, prepare derivative works, distribute copies to
// the public, perform publicly and display publicly, and to permit others to do
// so.

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/OpenCHAMI/jwtauth/v5"
	"github.com/lestrrat-go/jwx/jwk"
	"github.com/lestrrat-go/jwx/jwt"
)

var accessToken = ""

type OAuthClient struct {
	http.Client
	Id                      string
	Secret                  string
	RegistrationAccessToken string
	RedirectUris            []string
}

// This is to implement jwt.Clock and provide the Now() function. An empty
// instance of this struct will be passed to the jwt.WithClock() function so it
// knows how to verify the timestamps.
type nowClock struct {
	jwt.Clock
}

// This function returns whatever "now" is for jwt.Clock. We simply return
// time.Now(). It would be nice if we could just pass time.Now() to the
// jwt.WithClock function, but it forces us to have something that implements
// the jwt.Clock interface to do it.
func (nc nowClock) Now() time.Time {
	return time.Now()
}

// fetchPublicKey fetches the JWKS (JSON Key Set) needed to verify JWTs with issuer.
func fetchPublicKey(url string) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	set, err := jwk.Fetch(ctx, url)
	if err != nil {
		return fmt.Errorf("%v", err)
	}
	jwks, err := json.Marshal(set)
	if err != nil {
		return fmt.Errorf("failed to marshal JWKS: %v", err)
	}
	tokenAuth, err = jwtauth.NewKeySet(jwks)
	if err != nil {
		return fmt.Errorf("failed to initialize JWKS: %v", err)
	}

	// no errors occurred up to this point, so everything is fine here
	return nil
}

func (client *OAuthClient) CreateOAuthClient(registerUrl string) ([]byte, error) {
	// hydra endpoint: POST /clients
	data := []byte(`{
		"client_name":                "bss",
		"token_endpoint_auth_method": "client_secret_post",
		"scope":                      "openid email profile read",
		"grant_types":                ["client_credentials"],
		"response_types":             ["token"],
		"redirect_uris":               ["http://hydra:5555/callback"],
		"state":                      "12345678910"
	}`)

	req, err := http.NewRequest(http.MethodPost, registerUrl, bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %v", err)
	}
	req.Header.Add("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to do request: %v", err)
	}
	defer res.Body.Close()

	b, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %v", err)
	}
	// fmt.Printf("%v\n", string(b))
	var rjson map[string]any
	err = json.Unmarshal(b, &rjson)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal response body: %v", err)
	}
	// set the client ID and secret of registered client
	client.Id = rjson["client_id"].(string)
	client.Secret = rjson["client_secret"].(string)
	client.RegistrationAccessToken = rjson["registration_access_token"].(string)
	return b, nil
}

func (client *OAuthClient) PerformTokenGrant(remoteUrl string) (string, error) {
	// hydra endpoint: /oauth/token
	body := "grant_type=" + url.QueryEscape("client_credentials") +
		"&client_id=" + client.Id +
		"&client_secret=" + client.Secret +
		"&scope=read"
	headers := map[string][]string{
		"Content-Type":  {"application/x-www-form-urlencoded"},
		"Authorization": {"Bearer " + client.RegistrationAccessToken},
	}
	req, err := http.NewRequest(http.MethodPost, remoteUrl, bytes.NewBuffer([]byte(body)))
	req.Header = headers
	if err != nil {
		return "", fmt.Errorf("failed to make request: %s", err)
	}
	res, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to do request: %v", err)
	}
	defer res.Body.Close()

	b, err := io.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %v", err)
	}

	var rjson map[string]any
	err = json.Unmarshal(b, &rjson)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal response body: %v", err)
	}

	accessToken := rjson["access_token"]
	if accessToken == nil {
		return "", fmt.Errorf("no access token found")
	}

	return rjson["access_token"].(string), nil
}

func QuoteArrayStrings(arr []string) []string {
	for i, v := range arr {
		arr[i] = "\"" + v + "\""
	}
	return arr
}

// PollClientCreds tries retryCount times every retryInterval seconds to request
// client credentials and an access token (JWT) from the OAuth2 server. If
// attempts are exhausted or an invalid retryInterval is passed, an error is
// returned. If a JWT was successfully obtained, nil is returned.
func (client *OAuthClient) PollClientCreds(retryCount, retryInterval uint64) error {
	retryDuration, err := time.ParseDuration(fmt.Sprintf("%ds", retryInterval))
	if err != nil {
		return fmt.Errorf("Invalid retry interval: %v", err)
	}
	for i := uint64(0); i < retryCount; i++ {
		log.Printf("Attempting to obtain access token (attempt %d/%d)", i+1, retryCount)
		token, err := client.FetchAccessToken(oauth2AdminBaseURL + "/token")
		if err != nil {
			log.Printf("Failed to obtain client credentials and token: %v", err)
			time.Sleep(retryDuration)
			continue
		}
		log.Printf("Successfully obtained client credentials and token with %d attempts", i+1)
		accessToken = token
		return nil
	}
	log.Printf("Exhausted attempts to obtain client credentials and token")
	return fmt.Errorf("Exhausted %d attempts at obtaining client credentials and token", retryCount)
}

// JWTTestAndRefresh tests the current JWT. If either a parsing error occurs
// with it or the JWT is invalid, it attempts to fetch a new one. If all of this
// succeeds, nil is returned. Otherwise, an error is returned.
func (client *OAuthClient) JWTTestAndRefresh() (err error) {
	var (
		jwtIsValid bool
		reason     error
	)

	log.Printf("Validating JWT")
	if accessToken != "" {
		jwtIsValid, reason, err = JWTIsValid(accessToken)
		if err != nil {
			log.Printf("Unable to parse JWT, attempting to fetch a new one")
		} else if !jwtIsValid {
			log.Printf("JWT invalid, reason: %v", reason)
			log.Printf("Attempting to fetch a new one")
		} else {
			log.Printf("JWT is valid")
			return nil
		}
	} else {
		log.Printf("No JWT detected, fetching a new one")
	}

	err = client.PollClientCreds(authRetryCount, authRetryWait)
	if err != nil {
		log.Printf("Polling for OAuth2 client credentials failed")
		return fmt.Errorf("Failed to get access token: %v", err)
	}
	log.Printf("Successfully fetched new JWT")
	return nil
}

// JWTIsValid takes a string representing a JWT and validates that it is not
// expired. If the JWT is invalid (timestamp(s) is/are out of range), jwtValid
// is set to false, reason is set to the reason why the JWT is not valid, and
// err is nil.  If the JWT is valid (timestamps are all in range), jwtValid is
// set to true, reason is nil, and err is nil.
func JWTIsValid(jwtStr string) (jwtValid bool, reason, err error) {
	var token jwt.Token
	token, err = jwt.Parse([]byte(jwtStr))
	if err != nil {
		err = fmt.Errorf("failed to parse JWT string: %v", err)
		return
	}

	// Right now, we only validate the issued at, expiry, and not before
	// fields.
	// TODO: Add full validation.
	reason = jwt.Validate(token, jwt.WithClock(nowClock{}))
	debugf("JWT valid between %v and %v", token.NotBefore(), token.Expiration())
	debugf("Current time: %v", time.Now())
	if reason == nil {
		jwtValid = true
	} else {
		jwtValid = false
	}

	return
}

// FetchAccessToken fetches an access token for this client (BSS).
//
// Returns the access token string necessary to supply for authorization requests.
func (client *OAuthClient) FetchAccessToken(remoteUrl string) (string, error) {
	// opaal endpoint: /token
	headers := map[string][]string{
		"no-browser": {},
	}
	req, err := http.NewRequest(http.MethodPost, remoteUrl, nil)
	req.Header = headers
	if err != nil {
		return "", fmt.Errorf("failed to make request: %s", err)
	}
	res, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to do request: %v", err)
	}
	defer res.Body.Close()

	b, err := io.ReadAll(res.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response body: %v", err)
	}

	var rjson map[string]any
	err = json.Unmarshal(b, &rjson)
	if err != nil {
		return "", fmt.Errorf("failed to unmarshal response body: %v", err)
	}

	accessToken := rjson["access_token"]
	if accessToken == nil {
		return "", fmt.Errorf("no access token found")
	}

	return rjson["access_token"].(string), nil
}
