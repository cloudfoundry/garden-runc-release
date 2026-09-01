package csp

import (
	"net/http"
	"net/url"
	"strings"
)

type APITokenClient struct {
	BaseURL  string
	APIToken string
}

func (c *APITokenClient) GetAccessToken() (*AuthorizeResponse, error) {
	var oauthPath = "/csp/gateway/am/api/auth/api-tokens/authorize"
	client := &http.Client{}

	requestBody := url.Values{"api_token": {c.APIToken}}.Encode()
	req, err := http.NewRequest("POST", c.BaseURL+oauthPath, strings.NewReader(requestBody))

	if err != nil {
		return nil, err
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Accept", "application/json")

	resp, err := client.Do(req)

	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	return parseAuthorizeResponse(resp)
}
