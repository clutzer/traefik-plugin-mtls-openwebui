package main

import (
	"context"
	"net/http"
	"net/url"
	"strings"
)

// Config defines the configuration properties for the plugin.
type Config struct{}

// CreateConfig creates the default plugin configuration.
func CreateConfig() *Config {
	return &Config{}
}

// CertParser implements the Traefik middleware interface.
type CertParser struct {
	next http.Handler
	name string
}

// New instantiates a new instance of the plugin middleware.
func New(ctx context.Context, next http.Handler, config *Config, name string) (http.Handler, error) {
	return &CertParser{
		next: next,
		name: name,
	}, nil
}

func (a *CertParser) ServeHTTP(rw http.ResponseWriter, req *http.Request) {
	// 1. Security Safeguard: Strip existing identity headers to prevent spoofing
	req.Header.Del("X-User-Name")
	req.Header.Del("X-User-Email")

	// 2. Extract raw certificate metadata string injected by Traefik
	rawHeader := req.Header.Get("X-Forwarded-Tls-Client-Cert-Info")
	if rawHeader == "" {
		a.next.ServeHTTP(rw, req)
		return
	}

	// 3. Traefik URL-encodes this header natively. Decode it.
	decodedHeader, err := url.QueryUnescape(rawHeader)
	if err == nil {
		// Example format: Subject="CN=john.doe@example.com,OU=Engineering"
		if strings.Contains(decodedHeader, "CN=") {
			cnValue := extractCN(decodedHeader)
			
			if cnValue != "" {
				if strings.Contains(cnValue, "@") {
					// Format is an email address
					req.Header.Set("X-User-Email", cnValue)
					username := strings.Split(cnValue, "@")[0]
					req.Header.Set("X-User-Name", username)
				} else {
					// Fallback if CN is just a plain username string
					req.Header.Set("X-User-Name", cnValue)
				}
			}
		}
	}

	// 4. Pass control safely to the next middleware or backend service
	a.next.ServeHTTP(rw, req)
}

// Helper logic to cleanly extract the CN value from the complex Subject string
func extractCN(header string) string {
	start := strings.Index(header, "CN=")
	if start == -1 {
		return ""
	}
	start += 3 // Advance past the string length of "CN="

	remaining := header[start:]
	// The CN field ends at a comma separator or a trailing closing quote character
	end := strings.IndexAny(remaining, `,"`)
	if end == -1 {
		return remaining
	}
	return remaining[:end]
}
