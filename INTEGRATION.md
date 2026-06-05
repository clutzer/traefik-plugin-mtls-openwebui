# Integration Guide: traefik-plugin-mtls-open-webui

This document explains how to integrate the `traefik-plugin-mtls-open-webui` middleware into an existing Traefik-based project that already enforces mutual TLS (mTLS).

---

## What This Plugin Does

After Traefik's native `passTLSClientCert` middleware injects raw certificate metadata into the `X-Forwarded-Tls-Client-Cert-Info` header, this plugin:

1. Strips any pre-existing `X-User-Email` / `X-User-Name` headers (spoofing guard)
2. URL-decodes the Traefik certificate info header
3. Extracts the client's identity from the certificate
4. Injects clean `X-User-Email` and `X-User-Name` headers for downstream identity-aware services (e.g. Open WebUI)

Identity is extracted in this priority order:
- **Common Name (CN) from Subject** — if it contains `@`, treated as email
- **Subject Alternative Name (SAN)** — scans for email-like values as fallback
- If neither yields an email, `X-User-Name` is still set from the CN if present

---

## Prerequisites

- Traefik **v3.1+** (tested on v3.7.1)
- mTLS already configured via `tls.options` with `clientAuthType: RequireAndVerifyClientCert`
- `passTLSClientCert` middleware enabled on the target router
- The plugin source available locally (Git submodule, vendored copy, or copied into your repo)

---

## Step-by-Step Integration

### 1. Place the Plugin Source

The plugin must live in a path that Traefik's local plugin loader can discover. The convention is:

```
<project-root>/plugins-local/src/github.com/clutzer/traefik-plugin-mtls-open-webui/
```

Directory contents:
```
traefik-plugin-mtls-open-webui/
├── .traefik.yml          # Plugin manifest
├── go.mod                # Go module definition
└── main.go               # Plugin source
```

> **Package name requirement:** The `main.go` must declare `package traefik_plugin_mtls_open_webui` (hyphens from the import path become underscores). This is what Traefik's Yaegi interpreter expects. `package main` will fail with `undefined: traefik_plugin_mtls_open_webui`.

### 2. Mount the Plugin Source Into the Traefik Container

Add a read-only volume mount in your `docker-compose.yml` (or equivalent orchestration):

```yaml
services:
  gateway:
    image: traefik:v3
    volumes:
      # ... existing mounts ...
      - ./plugins-local/src/github.com/clutzer/traefik-plugin-mtls-open-webui/:/plugins-local/src/github.com/clutzer/traefik-plugin-mtls-open-webui/:ro
```

Traefik resolves the local plugin path from `/plugins-local/src/<moduleName>/`.

### 3. Register the Plugin in Traefik Static Configuration

Append the local plugin registration flag to Traefik's startup command:

```yaml
    command: >
      # ... existing flags ...
      --experimental.localplugins.cert-parser.modulename=github.com/clutzer/traefik-plugin-mtls-open-webui
```

> **Important:** Traefik v3.7.1 uses `--experimental.localplugins.*`, not `--localPlugins.*`. The plugin README refers to the latter, but the stable CLI flag in current releases is still under the `experimental` namespace.

Verify the plugin loads by checking Traefik logs for:
```
Loading plugins... plugins=["cert-parser"]
Plugins loaded. plugins=["cert-parser"]
```

### 4. Define the Plugin Middleware in Dynamic Configuration

Create or update a file served by Traefik's file provider (e.g. `traefik/dynamic.yml`):

```yaml
http:
  middlewares:
    cert-parser:
      plugin:
        cert-parser: {}
```

This declares a middleware named `cert-parser` that uses the plugin registered in static config.

### 5. Add the Plugin to the Router Middleware Chain

The `passTLSClientCert` middleware **must** execute before `cert-parser`, because the plugin depends on the `X-Forwarded-Tls-Client-Cert-Info` header that `passTLSClientCert` creates.

**Docker Compose labels example:**
```yaml
    labels:
      # ... router and TLS labels ...
      - traefik.http.routers.myapp.middlewares=myapp-pass-cert@docker,cert-parser@file
      - traefik.http.middlewares.myapp-pass-cert.passtlsclientcert.info.subject.commonname=true
      - traefik.http.middlewares.myapp-pass-cert.passtlsclientcert.info.sans=true
      # ... other passTLSClientCert info options as needed ...
```

**File-based router example:**
```yaml
http:
  routers:
    myapp:
      rule: "Host(`myapp.example.com`)"
      entryPoints:
        - websecure
      middlewares:
        - native-cert-extractor
        - cert-parser
      service: myapp
      tls:
        options: mymtls

  middlewares:
    native-cert-extractor:
      passTLSClientCert:
        info:
          subject:
            commonName: true
          sans: true

    cert-parser:
      plugin:
        cert-parser: {}
```

### 6. Verify End-to-End

With a valid client certificate, hit your endpoint and inspect the headers received by the backend. You should see:

| Header | Example Value | Source |
|---|---|---|
| `X-User-Name` | `clutzer` | CN or first part of SAN email |
| `X-User-Email` | `clutzer@akamai.com` | CN (if email format) or SAN fallback |

If the certificate's CN is a plain username (e.g. `CN=clutzer`) and the email lives in the SAN (e.g. `SAN="clutzer@akamai.com"`), the plugin still produces both headers correctly.

---

## Special Consideration: mtls-sample-app Template

If your project was created from or follows the **mtls-sample-app** template, much of the infrastructure is already in place. You only need to add the plugin-specific pieces.

### What's Already Configured

The template provides:

- `traefik/dynamic.yml` with `tls.options.mtls` enforcing `RequireAndVerifyClientCert`
- Docker Compose gateway service with `passTLSClientCert` middleware labels on the app router
- `ca/AkamaiClientCA.crt` mounted for client certificate verification
- Sample backend app that renders all incoming headers (ideal for plugin verification)

### What You Need to Add

1. **Plugin source** as a Git submodule (or copy) at `traefik-plugin-mtls-open-webui/plugins-local/src/github.com/clutzer/traefik-plugin-mtls-open-webui/`

2. **Volume mount** in `docker-compose.yml` gateway service:
   ```yaml
   volumes:
     - ./traefik-plugin-mtls-open-webui/plugins-local/src/github.com/clutzer/traefik-plugin-mtls-open-webui/:/plugins-local/src/github.com/clutzer/traefik-plugin-mtls-open-webui/:ro
   ```

3. **Static plugin registration** in the gateway `command:` block:
   ```
   --experimental.localplugins.cert-parser.modulename=github.com/clutzer/traefik-plugin-mtls-open-webui
   ```

4. **Dynamic middleware definition** in `traefik/dynamic.yml`:
   ```yaml
   http:
     middlewares:
       cert-parser:
         plugin:
           cert-parser: {}
   ```

5. **Middleware chain label** on the app router:
   ```yaml
   - traefik.http.routers.${APP_NAME}.middlewares=${APP_NAME}-pass-cert@docker,cert-parser@file
   ```

That's it. Restart the gateway service, load the app in your browser with a valid mTLS client certificate, and the sample app's diagnostics table will show `X-User-Email` and `X-User-Name` alongside the raw `X-Forwarded-Tls-Client-Cert-Info`.

### Quick Verification for Template Users

```bash
# Restart gateway to pick up plugin source changes
docker compose restart gateway

# Tail logs to confirm plugin load
docker compose logs gateway | grep "Plugins loaded"

# Then visit in a browser with your Akamai client certificate:
# https://<APP_NAME>.<APP_DOMAIN>
# Check the "All Request Headers" table for X-User-Email and X-User-Name.
```

---

## Troubleshooting

| Symptom | Likely Cause | Fix |
|---|---|---|
| `failed to decode configuration from flags: field not found, node: localPlugins` | Wrong CLI flag syntax for your Traefik version | Use `--experimental.localplugins...` |
| `undefined: traefik_plugin_mtls_open_webui` | `package main` instead of `package traefik_plugin_mtls_open_webui` | Update `main.go` package declaration |
| `Plugins are disabled because an error has occurred` | Yaegi compilation failure | Check Go syntax; ensure no external dependencies |
| `X-User-Email` missing, but `X-User-Name` present | Certificate CN has no `@` and SAN is empty or not forwarded | Enable `passtlsclientcert.info.sans=true` |
| `X-Forwarded-Tls-Client-Cert-Info` absent | `passTLSClientCert` not in middleware chain or disabled | Add/enable the native middleware before `cert-parser` |
| Headers don't update after `main.go` edit | Yaegi caches compiled plugins | Restart the Traefik container (not just reload) |

