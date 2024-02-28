package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// NOTE: Triad License goes here

type OAuthClient struct {
	http.Client
	Id           string
	Secret       string
	RedirectUris []string
}

func (client *OAuthClient) RegisterOAuthClient(registerUrl string, audience []string) ([]byte, error) {
	// hydra endpoint: POST /clients
	audience = QuoteArrayStrings(audience)
	data := []byte(fmt.Sprintf(`{
		"client_name":                "%s",
		"token_endpoint_auth_method": "client_secret_post",
		"scope":                      "openid email profile",
		"grant_types":                ["client_credentials", "urn:ietf:params:oauth:grant-type:jwt-bearer"],
		"response_types":             ["token"],
		"audience":                   [%s]
	}`, client.Id, client.Secret, strings.Join(audience, ",")))

	req, err := http.NewRequest("POST", registerUrl, bytes.NewBuffer(data))
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %v", err)
	}
	req.Header.Add("Content-Type", "application/json")
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to do request: %v", err)
	}
	defer res.Body.Close()

	return io.ReadAll(res.Body)
}

func (client *OAuthClient) FetchTokenFromAuthorizationServer(remoteUrl string, scope []string) ([]byte, error) {
	// hydra endpoint: /oauth/token
	data := "grant_type=" + url.QueryEscape("urn:ietf:params:oauth:grant-type:jwt-bearer") +
		"&client_id=" + client.Id +
		"&client_secret=" + client.Secret +
		"&scope=" + strings.Join(scope, "+")
	fmt.Printf("encoded params: %v\n\n", data)
	req, err := http.NewRequest("POST", remoteUrl, bytes.NewBuffer([]byte(data)))
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %s", err)
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to do request: %v", err)
	}
	defer res.Body.Close()

	return io.ReadAll(res.Body)
}

func QuoteArrayStrings(arr []string) []string {
	for i, v := range arr {
		arr[i] = "\"" + v + "\""
	}
	return arr
}
