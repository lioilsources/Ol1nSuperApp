package middleware

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type contextKey string

const UserEmailKey contextKey = "user"

type jwksCache struct {
	mu      sync.RWMutex
	keys    map[string]interface{}
	fetched time.Time
}

var cache jwksCache

func fetchJWKS(teamDomain string) (map[string]interface{}, error) {
	url := fmt.Sprintf("https://%s/cdn-cgi/access/certs", teamDomain)
	resp, err := http.Get(url) //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("jwks: fetch: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Keys []json.RawMessage `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("jwks: decode: %w", err)
	}

	keys := make(map[string]interface{})
	for _, raw := range result.Keys {
		var kid struct {
			Kid string `json:"kid"`
		}
		if err := json.Unmarshal(raw, &kid); err != nil {
			continue
		}
		keys[kid.Kid] = raw
	}
	return keys, nil
}

func Auth(teamDomain, lanKey string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// LAN key auth
			if lanKey != "" && r.Header.Get("X-LAN-Key") == lanKey {
				next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), UserEmailKey, "lan")))
				return
			}

			// CF Access JWT auth
			tokenStr := ""
			if cookie, err := r.Cookie("CF_Authorization"); err == nil {
				tokenStr = cookie.Value
			}
			if tokenStr == "" {
				tokenStr = strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			}
			if tokenStr == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			cache.mu.RLock()
			stale := time.Since(cache.fetched) > 10*time.Minute
			cache.mu.RUnlock()

			if stale {
				keys, err := fetchJWKS(teamDomain)
				if err != nil {
					slog.Error("jwks fetch failed", "err", err)
					http.Error(w, "auth error", http.StatusInternalServerError)
					return
				}
				cache.mu.Lock()
				cache.keys = keys
				cache.fetched = time.Now()
				cache.mu.Unlock()
			}

			token, err := jwt.Parse(tokenStr, func(t *jwt.Token) (interface{}, error) {
				if _, ok := t.Method.(*jwt.SigningMethodRSA); !ok {
					return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
				}
				kid, _ := t.Header["kid"].(string)
				cache.mu.RLock()
				raw, ok := cache.keys[kid]
				cache.mu.RUnlock()
				if !ok {
					return nil, fmt.Errorf("unknown kid: %s", kid)
				}
				return jwt.ParseRSAPublicKeyFromPEM([]byte(fmt.Sprintf("%s", raw)))
			})
			if err != nil || !token.Valid {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			claims, ok := token.Claims.(jwt.MapClaims)
			if !ok {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			email, _ := claims["email"].(string)
			next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), UserEmailKey, email)))
		})
	}
}
