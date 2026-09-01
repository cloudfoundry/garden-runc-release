package auth

import "net/http"

type WavefrontTokenService struct {
	Token string
}

func (t WavefrontTokenService) IsDirect() bool {
	return true
}

func (t WavefrontTokenService) Authorize(req *http.Request) error {
	req.Header.Set("Authorization", "Bearer "+t.Token)
	return nil
}

func (t WavefrontTokenService) Close() {
}

// NewWavefrontTokenService returns a Service instance where it always returns a Wavefront API Token
func NewWavefrontTokenService(Token string) Service {
	return &WavefrontTokenService{Token: Token}
}
