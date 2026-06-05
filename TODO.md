# TODO

## Critical — Plugin will not function

- [x] ~~**Fix Traefik v3.1 local plugin flag syntax**~~
  - `docker-compose.yml` already uses `--experimental.localplugins...` which is the **correct** Traefik v3.x syntax for local plugins.
  - The `--experimental` prefix is required — there is no non-experimental `--localPlugins` CLI flag.
  - ✅ No change needed.

- [ ] **Add mTLS `clientAuth` TLS options**
  - `dynamic-config.yml` references `tls: options: default`
  - Must define a custom TLS option (e.g. `mymtls`) with `clientAuthType: RequireAndVerifyClientCert` and a mounted CA file
  - Without this, Traefik never requests client certs and the plugin receives an empty header on every request

- [ ] **Fix backend service URL**
  - `dynamic-config.yml` points to `http://127.0.0.1:8080`
  - Inside the container this is Traefik's own dashboard; creates a self-referential loop
  - Change to the actual Open WebUI container name (e.g. `http://open-webui:8080`)

- [ ] **Add Open WebUI service to `docker-compose.yml`**
  - The compose file only defines Traefik; there is no backend to route to
  - Add an `open-webui` service, or at minimum a test/mock backend, and put both services on a shared Docker network

- [ ] **Fix port collision**
  - Traefik dashboard is mapped to host port `8080`
  - If the backend also runs on `8080`, move the dashboard to `8081:8080` or assign a different host port to the backend

## Major — Reliability & Maintainability

- [ ] **Add structured error logging**
  - Every failure path in `main.go` is currently silent
  - Log: missing cert header, URL-decode failure, missing CN, empty CN value
  - Use `log` package or Traefik's logger

- [ ] **Handle requests without a certificate**
  - Currently requests missing the cert header pass through unauthenticated with no error
  - Decide policy: return `403 Forbidden`, or leave enforcement strictly to the TLS layer
  - If leaving it to TLS, document that clearly

- [ ] **Improve CN parsing robustness**
  - `extractCN` splits naively on the first `,` or `"`
  - Does not handle RFC 4514 escaped commas (`\,`) inside the CN value
  - Does not trim whitespace around values
  - Consider parsing with `crypto/x509/pkix` if the raw cert is available instead of string parsing

- [ ] **Add unit tests**
  - Test `extractCN` with various inputs (email CN, plain CN, CN with commas, missing CN, empty CN)
  - Test `ServeHTTP` spoofing guard (incoming `X-User-*` headers are stripped)
  - Test `ServeHTTP` happy paths and edge cases
  - Add a `*_test.go` file

- [ ] **Add integration test for Traefik + plugin**
  - Spin up Traefik with the plugin in a testcontainer or temporary Docker Compose stack
  - Send a request with a crafted `X-Forwarded-Tls-Client-Cert-Info` header and assert output headers

## Minor — Polish

- [ ] **Bump `go.mod` Go version**
  - Currently `go 1.21`
  - Traefik v3.1 is built with Go 1.22+; matching that version reduces Yaegi compatibility risk

- [ ] **Make output headers configurable**
  - Add exported fields to `Config{}` so users can customize `EmailHeader` and `NameHeader`
  - Update `.traefik.yml` `testData` with sample configuration

- [ ] **Add plugin `icon` and metadata to `.traefik.yml`**
  - Consider adding `icon`, `version`, `compatibility` fields for the Traefik plugin catalog

- [ ] **Verify actual Traefik `passTLSClientCert` output format**
  - The code assumes `Subject="CN=..."` wrapped in quotes
  - Confirm against a live Traefik v3.1 instance with a real client certificate

- [ ] **Document Open WebUI trusted header setup**
  - `README.md` describes the `WEBUI_AUTH_TRUSTED_EMAIL_HEADER` env var
  - Verify this is the correct variable for the user's Open WebUI version
