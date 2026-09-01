package auth

import "net/http"

// Service Interface for getting authentication tokens (Wavefront, CSP)
type Service interface {
	Authorize(r *http.Request) error
	Close()
	IsDirect() bool
}

type APIToken struct {
	Token string
}

type CSPClientCredentials struct {
	ClientID     string
	ClientSecret string
	BaseURL      string
	OrgID        *string
}

type CSPAPIToken struct {
	Token   string
	BaseURL string
}
