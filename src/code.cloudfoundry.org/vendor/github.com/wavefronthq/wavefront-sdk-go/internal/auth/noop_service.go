package auth

import (
	"net/http"
)

var (
	defaultNoopService Service = &NoOpService{}
)

type NoOpService struct {
}

func (t NoOpService) IsDirect() bool {
	return false
}

func (t NoOpService) Authorize(*http.Request) error {
	return nil
}

func (t NoOpService) Close() {
}

// NewNoopTokenService returns a Service instance where it always returns an empty string for the token (for proxy usage).
func NewNoopTokenService() Service {
	return defaultNoopService
}
