package senders

import (
	"log"

	"github.com/wavefronthq/wavefront-sdk-go/internal/auth"
)

func tokenServiceForCfg(cfg *configuration) auth.Service {
	switch cfg.Authentication.(type) {
	case auth.APIToken:
		log.Println("The Wavefront SDK will use Direct Ingestion authenticated using an API Token.")
		tokenAuth := cfg.Authentication.(auth.APIToken)
		return auth.NewWavefrontTokenService(tokenAuth.Token)
	case auth.CSPClientCredentials:
		log.Println("The Wavefront SDK will use Direct Ingestion authenticated using CSP client credentials.")
		cspAuth := cfg.Authentication.(auth.CSPClientCredentials)
		return auth.NewCSPServerToServerService(cspAuth.BaseURL, cspAuth.ClientID, cspAuth.ClientSecret, cspAuth.OrgID)
	case auth.CSPAPIToken:
		log.Println("The Wavefront SDK will use Direct Ingestion authenticated using CSP API Token.")
		cspAuth := cfg.Authentication.(auth.CSPAPIToken)
		return auth.NewCSPTokenService(cspAuth.BaseURL, cspAuth.Token)
	}

	log.Println("The Wavefront SDK will communicate with a Wavefront Proxy.")
	return auth.NewNoopTokenService()
}
