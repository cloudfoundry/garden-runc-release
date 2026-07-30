package csp

import (
	"encoding/base64"
	"net/http"
	"net/url"
	"strings"
)

type ClientCredentialsClient struct {
	BaseURL      string
	ClientID     string
	ClientSecret string
	OrgID        *string
}

func (c *ClientCredentialsClient) authHeaderValue() string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(c.ClientID+":"+c.ClientSecret))
}

func (c *ClientCredentialsClient) GetAccessToken() (*AuthorizeResponse, error) {
	var oauthPath = "/csp/gateway/am/api/auth/authorize"
	client := &http.Client{}

	values := url.Values{"grant_type": {"client_credentials"}}
	if c.OrgID != nil {
		values.Add("orgId", *c.OrgID)
	}
	requestBody := values.Encode()
	req, err := http.NewRequest("POST", c.BaseURL+oauthPath, strings.NewReader(requestBody))
	if err != nil {
		return nil, err
	}

	req.Header.Add("Authorization", c.authHeaderValue())
	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	defer resp.Body.Close()

	return parseAuthorizeResponse(resp)
}
