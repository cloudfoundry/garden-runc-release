package csp

type Client interface {
	GetAccessToken() (*AuthorizeResponse, error)
}

type AuthorizeResponse struct {
	IDToken     string `json:"id_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
	Scope       string `json:"scope"`
	AccessToken string `json:"access_token"`
}
