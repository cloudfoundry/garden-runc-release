package csp

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func parseAuthorizeResponse(resp *http.Response) (*AuthorizeResponse, error) {
	if resp.StatusCode > 399 {
		return nil, fmt.Errorf("authentication failed: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var cspResponse AuthorizeResponse
	err = json.Unmarshal(body, &cspResponse)
	if err != nil {
		return nil, err
	}

	return &cspResponse, nil
}
