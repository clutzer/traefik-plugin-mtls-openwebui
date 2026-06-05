# traefik-plugin-mtls-open-webui

A Traefik middleware plugin that bridges Mutual TLS (mTLS) client certificate authentication to [Open WebUI](https://github.com/open-webui/open-webui) trusted header authentication.

## What it does

When a client presents a valid client certificate, Traefik's native `passTLSClientCert` middleware extracts the certificate subject information and injects it into the `X-Forwarded-Tls-Client-Cert-Info` header. This plugin:

1. Strips any pre-existing `X-User-*` identity headers (spoofing guard)
2. URL-decodes the Traefik certificate info header
3. Extracts the certificate's Common Name (CN)
4. Sets `X-User-Email` and `X-User-Name` headers for Open WebUI's trusted header authentication

## Data Flow

```
Client (mTLS cert)
  → Traefik (TLS termination + passTLSClientCert)
    → this plugin (parses CN)
      → Open WebUI (trusted header auth)
```

## Requirements

- Traefik **v3.1+** (this plugin uses the v3 local plugin syntax)
- A CA certificate configured in Traefik to verify client certificates
- Open WebUI configured for trusted header authentication via `X-User-Email`

## Directory Layout

```
traefik-plugin-mtls-open-webui/
├── docker-compose.yml          # Local dev stack
├── dynamic-config.yml          # Traefik dynamic routing & middleware config
└── plugins-local/
    └── src/
        └── github.com/
            └── clutzer/
                └── traefik-plugin-mtls-open-webui/
                    ├── .traefik.yml   # Plugin manifest
                    ├── go.mod         # Go module definition
                    └── main.go        # Plugin source
```

## Local Plugin Registration (Traefik v3.1+)

Register the plugin in Traefik's **static** config:

```yaml
# traefik.yml (static)
localPlugins:
  cert-parser:
    moduleName: github.com/clutzer/traefik-plugin-mtls-open-webui
```

Or via CLI flag in `docker-compose.yml`:

```yaml
services:
  traefik:
    command:
      - "--localPlugins.cert-parser.modulename=github.com/clutzer/traefik-plugin-mtls-open-webui"
```

> ⚠️ Do **not** use `--experimental.localplugins`. That is Traefik v2 syntax and will not work with v3.1.

## Traefik Dynamic Configuration

```yaml
http:
  routers:
    open-webui:
      rule: "Host(`webui.example.com`)"
      entryPoints:
        - websecure
      middlewares:
        - native-cert-extractor
        - cert-parser
      service: open-webui
      tls:
        options: mymtls               # ← must require client certs

  middlewares:
    native-cert-extractor:
      passTLSClientCert:
        info:
          subject:
            commonName: true

    cert-parser:
      plugin:
        cert-parser: {}

  services:
    open-webui:
      loadBalancer:
        servers:
          - url: "http://open-webui:8080"
```

### mTLS TLS Options (Critical)

The router **must** reference TLS options that require and verify client certificates. Without this, Traefik will not request a certificate during the handshake and `X-Forwarded-Tls-Client-Cert-Info` will be empty.

```yaml
tls:
  options:
    mymtls:
      clientAuth:
        caFiles:
          - /etc/traefik/certs/ca.crt
        clientAuthType: RequireAndVerifyClientCert
```

## Open WebUI Configuration

Enable trusted header authentication and point it at the header this plugin produces:

```bash
# docker-compose.yml environment for Open WebUI
environment:
  - WEBUI_AUTH_TRUSTED_EMAIL_HEADER=X-User-Email
```

Open WebUI will use the email to automatically create or log in the user. `X-User-Name` is optional and used for display purposes.

## Security Notes

- The plugin explicitly **deletes** incoming `X-User-Name` and `X-User-Email` headers before processing. This prevents external header spoofing.
- mTLS enforcement must happen at the **TLS layer** (via `clientAuthType: RequireAndVerifyClientCert`). Relying solely on this middleware for authorization is insufficient because requests without a certificate simply pass through with no identity headers set.

## Development

The plugin uses only Go standard library packages and is designed to run inside Traefik's embedded [Yaegi](https://github.com/traefik/yaegi) interpreter. No external dependencies are required.

### Hot Reload Caveats

- Changes to `dynamic-config.yml` are picked up automatically by Traefik's file provider watcher.
- Changes to `main.go` require a **full container restart** so Yaegi can re-parse the plugin source.

```bash
docker compose restart traefik
```

### Testing

There are currently no automated tests. See `TODO.md` for planned test coverage.
