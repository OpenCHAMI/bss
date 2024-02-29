// NOTE: Triad License goes here
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

type OAuthClient struct {
	http.Client
	Id                      string
	Secret                  string
	RegistrationAccessToken string
	RedirectUris            []string
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

func (client *OAuthClient) AuthorizeOAuthClient(authorizeUrl string) ([]byte, error) {
	// encode ID and secret for authorization header basic authentication
	// basicAuth := base64.StdEncoding.EncodeToString(
	// 	[]byte(fmt.Sprintf("%s:%s",
	// 		url.QueryEscape(client.Id),
	// 		url.QueryEscape(client.Secret),
	// 	)),
	// )
	body := []byte("grant_type=client_credentials&scope=read&client_id=" + client.Id +
		"&client_secret=" + client.Secret +
		"&redirect_uri=" + url.QueryEscape("http://hydra:5555/callback") +
		"&response_type=token" +
		"&state=12345678910",
	)
	headers := map[string][]string{
		"Authorization": {"Bearer " + client.RegistrationAccessToken},
		"Content-Type":  {"application/x-www-form-urlencoded"},
	}

	req, err := http.NewRequest("POST", authorizeUrl, bytes.NewBuffer(body))
	req.Header = headers
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %v", err)
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to do request: %v", err)
	}
	defer res.Body.Close()

	return io.ReadAll(res.Body)
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
	req, err := http.NewRequest("POST", remoteUrl, bytes.NewBuffer([]byte(body)))
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

	return rjson["access_token"].(string), nil
}

func QuoteArrayStrings(arr []string) []string {
	for i, v := range arr {
		arr[i] = "\"" + v + "\""
	}
	return arr
}
