package traefik_plugin_mtls_open_webui

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
	if err != nil {
		a.next.ServeHTTP(rw, req)
		return
	}

	// 4. Extract CN from Subject, then fall back to SAN for email
	cnValue := extractSubjectCN(decodedHeader)
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

	// If CN didn't provide an email, try SAN
	if req.Header.Get("X-User-Email") == "" {
		sanEmails := extractSANEmails(decodedHeader)
		for _, email := range sanEmails {
			req.Header.Set("X-User-Email", email)
			if req.Header.Get("X-User-Name") == "" {
				username := strings.Split(email, "@")[0]
				req.Header.Set("X-User-Name", username)
			}
			break // Use first email SAN found
		}
	}

	// 5. Pass control safely to the next middleware or backend service
	a.next.ServeHTTP(rw, req)
}

// extractSubjectCN isolates the Subject="..." field, then extracts CN from it.
// This prevents accidentally matching the Issuer's CN.
func extractSubjectCN(header string) string {
	subjectPrefix := `Subject="`
	start := strings.Index(header, subjectPrefix)
	if start == -1 {
		return ""
	}
	start += len(subjectPrefix)
	end := strings.Index(header[start:], `"`)
	if end == -1 {
		return ""
	}
	subject := header[start : start+end]
	return extractCN(subject)
}

// extractCN extracts the CN value from a string containing CN=...
func extractCN(s string) string {
	start := strings.Index(s, "CN=")
	if start == -1 {
		return ""
	}
	start += 3 // Advance past the string length of "CN="

	remaining := s[start:]
	// The CN field ends at a comma separator or a trailing quote character
	end := strings.IndexAny(remaining, `, "`)
	if end == -1 {
		return remaining
	}
	return remaining[:end]
}

// extractSANEmails pulls email-looking values from the SAN field.
func extractSANEmails(header string) []string {
	prefix := `SAN="`
	start := strings.Index(header, prefix)
	if start == -1 {
		return nil
	}
	start += len(prefix)
	end := strings.Index(header[start:], `"`)
	if end == -1 {
		return nil
	}
	sanValue := header[start : start+end]

	var emails []string
	for _, san := range strings.Split(sanValue, ",") {
		san = strings.TrimSpace(san)
		// Strip known SAN type prefixes if Traefik includes them
		san = strings.TrimPrefix(san, "email:")
		san = strings.TrimPrefix(san, "rfc822Name:")
		san = strings.TrimSpace(san)
		if strings.Contains(san, "@") {
			emails = append(emails, san)
		}
	}
	return emails
}
