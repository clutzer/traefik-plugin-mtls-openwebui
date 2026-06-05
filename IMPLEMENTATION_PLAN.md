# Implementation Plan: `traefik-plugin-mtls-openwebui`

## 1. Project Overview
The objective is to build a Traefik middleware plugin written in Go that acts as a bridge between Mutual TLS (mTLS) client certificate authentication and Open WebUI's trusted header authentication. 

The plugin will run inside an unmodified, stock Traefik Docker container utilizing Traefik's embedded Go interpreter (**Yaegi**) via the Local Plugins feature.

### Data Flow Pipeline:
1. **Client** initiates HTTPS request with a client certificate.
2. **Traefik Native Middleware (`passTLSClientCert`)** validates the certificate and injects a URL-encoded string into the `X-Forwarded-Tls-Client-Cert-Info` header.
3. **This Custom Plugin** intercepts the request, sanitizes the environment, URL-decodes the header string, extracts the Common Name (CN), parses the Username and Email, and sets `X-User-Name` and `X-User-Email`.
4. **Open WebUI** consumes `X-User-Email` via its `trusted-header` auth mechanism.

## 2. Directory & Architecture Specification
The Agent must construct the following file tree layout on the host machine. This structure mimics standard Go packaging required by Traefik's local plugin loader engine.

```text
traefik-plugin-mtls-openwebui/
├── IMPLEMENTATION_PLAN.md  (This file)
├── dynamic-config.yml       # Traefik dynamic routing configuration
├── docker-compose.yml       # Local integration testing environment
└── plugins-local/
    └── src/
        └── [github.com/](https://github.com/)
            └── clutzer/
                └── traefik-plugin-mtls-openwebui/
                    ├── .traefik.yml   # Traefik Plugin Manifest
                    ├── go.mod         # Go module definition
                    └── main.go        # Plugin source code
```

## 3. Step-by-Step Execution Phases

### Phase 1: Initialize the Go Module & Manifest
Navigate to the directory plugins-local/src/github.com/clutzer/traefik-plugin-mtls-openwebui/ and create the core configuration files.

#### Task 1.1: Create go.mod
Initialize the module precisely matching the local import path layout.

```
module github.com/clutzer/traefik-plugin-mtls-openwebui

go 1.21
```


#### Task 1.2: Create .traefik.yml
This manifest defines metadata and provides mock configuration settings for Traefik's startup validator.

```
displayName: "mTLS to Open WebUI Header Parser"
type: "middleware"
import: "github.com/clutzer/traefik-plugin-mtls-openwebui"
summary: "Decodes Traefik mTLS certificate info into X-User-Name and X-User-Email headers for Open WebUI."

testData: {}


---
```

### Phase 2: Core Plugin Logic (main.go)
Create main.go inside the root of the module folder. 

```
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
```

#### Requirements & Quirks to Implement:
* Zero Dependencies: Use only the Go standard library (strings, net/url, net/http, context). Do not import external packages.
* Security Sanitization: Explicitly drop/delete X-User-Name and X-User-Email from the incoming request *before* executing parsing logic. This prevents external header spoofing.
* URL Decoding: The native X-Forwarded-Tls-Client-Cert-Info string is URL-encoded by Traefik. It must be decoded using url.QueryUnescape().
* CN Parsing Syntax: Extract the value following CN=. Handle strings terminated by either commas (e.g., CN=john@example.com,OU=Dev) or quotes.
* Email Extraction: If the CN contains an @ symbol, map the full CN to X-User-Email, split the prefix before @, and map that prefix to X-User-Name.

#### Task 2.1: Write main.go

### Phase 3: Infrastructure Integration & Test Orchestration

#### Task 3.1: Create docker-compose.yml
Configure a stock, official Traefik image container. Ensure the volume mapping mounts the local source directories to the container root filesystem (`/plugins-local`). Use command line arguments to register the module statically inside the experimental runtime environment.

```
version: '3.8'

services:
  traefik:
    image: traefik:v3.1
    container_name: traefik-mtls-dev
    ports:
      - "443:443"
      - "8080:8080" # Traefik Web UI Dashboard
    command:
      - "--api.insecure=true"
      - "--providers.docker=true"
      - "--providers.docker.exposedbydefault=false"
      - "--entrypoints.websecure.address=:443"
      # Enable File Provider for custom dynamic configurations
      - "--providers.file.filename=/dynamic-config.yml"
      - "--providers.file.watch=true"
      # Register our Local Plugin statically inside Traefik
      - "--experimental.localplugins.cert-parser.modulename=github.com/clutzer/traefik-plugin-mtls-open-webui"
    volumes:
      - /var/run/docker.sock:/var/run/docker.sock:ro
      - ./dynamic-config.yml:/dynamic-config.yml:ro
      # Crucial: Local code mounting directly to container runtime space
      - ./plugins-local:/plugins-local:ro
```

#### Task 3.2: Create dynamic-config.yml
Construct the routing pipeline mapping the sequential execution chain: Native Traefik Client Cert extraction -> Your Custom Local Decoupling Plugin -> Application Backend.

```
http:
  routers:
    open-webui-route:
      rule: "Host(`localhost`)"
      entryPoints:
        - websecure
      middlewares:
        - native-cert-extractor
        - execute-custom-parser-plugin
      service: open-webui-mock-service
      tls:
        options: default

  middlewares:
    # Step 1: Tell Traefik to look at client handshake data and put it into headers
    native-cert-extractor:
      passTLSClientCert:
        info:
          subject:
            commonName: true

    # Step 2: Pass control to our custom compiled code block
    execute-custom-parser-plugin:
      plugin:
        cert-parser: {} # Activates local plugin matching static declaration

  services:
    open-webui-mock-service:
      loadBalancer:
        servers:
          - url: "http://127.0.0.1:8080" # Replace with actual Open WebUI container address later
```

## 4. Testing, Diagnostics & Maintenance Rules

* Hot Reload Constraints: The Agent must keep in mind that editing dynamic-config.yml refreshes instantly, but editing code changes inside main.go requires a full restart of the docker container (docker compose restart traefik) so the built-in Yaegi engine can re-parse the codebase.
* Log Inspection: The Agent can track parsing errors or syntax failures by executing "docker logs traefik-mtls-dev". Code failures will appear immediately during boot-up compilation inside stdout logs.
