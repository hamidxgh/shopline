package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/coreos/go-oidc/v3/oidc"
)

type authenticator struct {
	verifier *oidc.IDTokenVerifier
}

func newAuthenticator(ctx context.Context) (*authenticator, error) {
	issuer := os.Getenv("KEYCLOAK_ISSUER")
	if issuer == "" {
		issuer = "http://localhost:8084/realms/shopline"
	}
	discovery := os.Getenv("KEYCLOAK_DISCOVERY")
	if discovery == "" {
		discovery = issuer
	}

	// Fetch OIDC metadata/JWKS from the in-network URL.
	// Tokens minted via host curl carry iss=localhost:8084, so skip iss check in the lab.
	_ = issuer
	provider, err := oidc.NewProvider(ctx, discovery)
	if err != nil {
		return nil, err
	}
	return &authenticator{
		verifier: provider.Verifier(&oidc.Config{
			SkipClientIDCheck: true,
			SkipIssuerCheck:   true,
		}),
	}, nil
}

func (a *authenticator) middleware(next http.Handler) http.Handler {
	skip := map[string]bool{
		"GET /healthz": true,
		"GET /metrics": true,
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.Method + " " + r.URL.Path
		if r.Pattern != "" {
			key = r.Pattern
		}
		if skip[key] || r.URL.Path == "/healthz" || r.URL.Path == "/metrics" {
			next.ServeHTTP(w, r)
			return
		}
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			http.Error(w, "missing bearer token", http.StatusUnauthorized)
			return
		}
		raw := strings.TrimPrefix(h, "Bearer ")
		if _, err := a.verifier.Verify(r.Context(), raw); err != nil {
			log.Printf("auth: %v", err)
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}
