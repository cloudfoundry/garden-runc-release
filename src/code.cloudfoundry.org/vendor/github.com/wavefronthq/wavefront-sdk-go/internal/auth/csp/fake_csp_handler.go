package csp

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
)

func FakeCSPHandler(apiTokens []string) http.Handler {
	basicAuthCredentials := "Basic " + base64.StdEncoding.EncodeToString([]byte("a:b"))
	firstRun := true

	mux := http.NewServeMux()
	mux.HandleFunc("/csp/gateway/am/api/auth/authorize", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.Header.Get("Authorization"), basicAuthCredentials) {
			var sup AuthorizeResponse

			if firstRun {
				sup = AuthorizeResponse{
					ExpiresIn:   1,
					AccessToken: "abc",
					Scope:       "aoa:directDataIngestion",
				}
				firstRun = false
			} else {
				sup = AuthorizeResponse{
					ExpiresIn:   1,
					AccessToken: "def",
					Scope:       "aoa:directDataIngestion",
				}
			}

			w.WriteHeader(http.StatusOK)
			marshal, _ := json.Marshal(sup)
			_, _ = w.Write(marshal)
			return
		}
		w.WriteHeader(http.StatusUnauthorized)
	})
	mux.HandleFunc("/csp/gateway/am/api/auth/api-tokens/authorize", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			w.WriteHeader(http.StatusNotAcceptable)
			return
		}
		if !(r.Form.Has("api_token")) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var tokenMatch = false
		for _, token := range apiTokens {
			if r.Form.Get("api_token") == token {
				tokenMatch = true
				break
			}
		}

		if !(tokenMatch) {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		var sup AuthorizeResponse
		if firstRun {
			sup = AuthorizeResponse{
				ExpiresIn:   1,
				AccessToken: "abc",
				Scope:       "aoa:directDataIngestion",
			}
			firstRun = false
		} else {
			sup = AuthorizeResponse{
				ExpiresIn:   1,
				AccessToken: "def",
				Scope:       "aoa:directDataIngestion",
			}
		}

		w.WriteHeader(http.StatusOK)
		marshal, _ := json.Marshal(sup)
		_, _ = w.Write(marshal)
	})
	return mux
}
