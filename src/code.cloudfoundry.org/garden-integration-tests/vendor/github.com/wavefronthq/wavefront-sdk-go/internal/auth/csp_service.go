package auth

import (
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/wavefronthq/wavefront-sdk-go/internal/auth/csp"
)

type tokenResult struct {
	accessToken string
	err         error
}

type CSPService struct {
	client                 csp.Client
	mutex                  sync.Mutex
	tokenResult            *tokenResult
	refreshTicker          *time.Ticker
	done                   chan bool
	defaultRefreshInterval time.Duration
}

// NewCSPServerToServerService returns a Service instance that gets access tokens via CSP client credentials
func NewCSPServerToServerService(
	CSPBaseURL string,
	ClientID string,
	ClientSecret string,
	OrgID *string,
) Service {
	return newService(&csp.ClientCredentialsClient{
		BaseURL:      CSPBaseURL,
		ClientID:     ClientID,
		ClientSecret: ClientSecret,
		OrgID:        OrgID,
	})
}

func NewCSPTokenService(CSPBaseURL, apiToken string) Service {
	return newService(&csp.APITokenClient{
		BaseURL:  CSPBaseURL,
		APIToken: apiToken,
	})
}

func newService(client csp.Client) Service {
	return &CSPService{
		client:                 client,
		defaultRefreshInterval: 60 * time.Second,
	}
}

func (s *CSPService) IsDirect() bool {
	return true
}

func (s *CSPService) Authorize(r *http.Request) error {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if s.tokenResult == nil {
		s.RefreshAccessToken()
	}

	if s.tokenResult.err != nil {
		return &Err{
			error: s.tokenResult.err,
		}
	}

	r.Header.Set("Authorization", "Bearer "+s.tokenResult.accessToken)
	return nil
}

func (s *CSPService) RefreshAccessToken() {
	cspResponse, err := s.client.GetAccessToken()

	if err != nil {
		s.tokenResult = &tokenResult{
			accessToken: "",
			err:         err,
		}
		s.scheduleNextTokenRefresh(s.defaultRefreshInterval)
		return
	}

	s.scheduleNextTokenRefresh(time.Duration(cspResponse.ExpiresIn) * time.Second)
	s.tokenResult = &tokenResult{
		accessToken: cspResponse.AccessToken,
		err:         nil,
	}
}

func (s *CSPService) scheduleNextTokenRefresh(expiresIn time.Duration) {
	tickerInterval := calculateNewTickerInterval(expiresIn, s.defaultRefreshInterval)

	if s.refreshTicker == nil {
		s.refreshTicker = time.NewTicker(tickerInterval)
		s.done = make(chan bool)
		go func() {
			for {
				select {
				case <-s.done:
					return
				case tick := <-s.refreshTicker.C:
					s.mutex.Lock()
					log.Printf("Re-fetching CSP credentials at: %v \n", tick)
					s.RefreshAccessToken()
					s.mutex.Unlock()
				}
			}
		}()
	} else {
		s.refreshTicker.Reset(tickerInterval)
	}
}

func (s *CSPService) Close() {
	log.Println("Shutting down the CSPService")
	if s.refreshTicker == nil {
		return
	}
	s.done <- true
}
