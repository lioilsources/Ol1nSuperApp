# ol1n-vault project context

Backend service pro Ol1n Vault — UseNet archivace s krypto platbami.

## Stack
- Go 1.23, chi router, `modernc.org/sqlite` (pure Go, bez CGO)
- NNTP/TLS protokol → Eweka UseNet provider
- NOWPayments API pro platby (HMAC-SHA512 webhook verifikace)
- Polygon blockchain pro volitelnou notarizaci (zatím stub)

## Deploy target
NAS Ubuntu x86_64, 4GB RAM, systemd service, za Cloudflare Tunnel + Caddy

## Kritická pravidla
- NOWPayments webhook: vždy číst raw body před json parserem
- JSON klíče MUSÍ být seřazeny před HMAC-SHA512 výpočtem
- SQLite UPDATE jobu po platbě musí být idempotentní (WHERE status='pending_payment')
- NNTP spojení: pool max 8 connections na Eweka
- Soubory v /data/vault/files/ mají TTL 24h — čistit přes ticker
- Webhook endpoint `/upload/payment-confirm` je veřejný (NOWPayments volá bez auth)
- Všechny ostatní endpointy chráněny `X-Vault-Key` header (VAULT_API_KEY)

## Odpovídej česky.
