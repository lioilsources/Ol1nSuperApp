# Ol1n Vault — implementační plán

> Součást projektu **SuperOl1n** | Backend: `super.ol1n.com` | Hardware: NAS Ubuntu (Celeron, 18TB)

---

## Obsah

1. [Přehled architektury](#1-přehled-architektury)
2. [Infrastruktura: Cloudflare + Caddy + cloudflared](#2-infrastruktura-cloudflare--caddy--cloudflared)
3. [Go backend — struktura projektu](#3-go-backend--struktura-projektu)
4. [SQLite schéma](#4-sqlite-schéma)
5. [Upload pipeline](#5-upload-pipeline)
6. [Download pipeline](#6-download-pipeline)
7. [NOWPayments integrace](#7-nowpayments-integrace)
8. [Blockchain notarizace (volitelná)](#8-blockchain-notarizace-volitelná)
9. [Flutter integrace](#9-flutter-integrace)
10. [Deployment na NAS](#10-deployment-na-nas)
11. [Doporučení pro Claude Code](#11-doporučení-pro-claude-code)

---

## 1. Přehled architektury

```
Flutter (SuperOl1n)
    │
    │  HTTPS
    ▼
Cloudflare Edge (DNS + WAF + DDoS)
    │
    │  Cloudflare Tunnel (cloudflared)
    ▼
NAS Ubuntu — Caddy reverse proxy :443
    │
    ├── /upload/*   ──► Go: ol1n-vault  :8090
    ├── /download/* ──► Go: ol1n-vault  :8090
    ├── /files/*    ──► static file server (Caddy)
    └── /health     ──► Go: ol1n-vault  :8090

Go backend (ol1n-vault)
    ├── SQLite DB       /data/vault/vault.db
    ├── NZB cache       /data/vault/nzb/
    ├── Download cache  /data/vault/files/  (dočasné, TTL čištění)
    └── goroutine pool  (upload jobs + download jobs)

Externí služby
    ├── Eweka UseNet   — NNTP upload + download
    ├── NOWPayments    — krypto platební gateway
    └── Polygon RPC    — blockchain notarizace (volitelné)
```

**Hardware rozdělení:**
- **NAS** (Celeron, 4GB RAM, 18TB): veškerý vault backend, SQLite, NNTP I/O, file cache
- **DGX Spark**: nedotčen — pouze LLM inference
- **MacBook / Mac Mini**: vývoj

---

## 2. Infrastruktura: Cloudflare + Caddy + cloudflared

### 2.1 Cloudflare DNS

```
# DNS záznamy (Cloudflare dashboard → DNS)
Type  Name   Content           Proxy
CNAME super  <tunnel-id>.cfargotunnel.com   ✓ (oranžový mrak)
```

Doména `super.ol1n.com` → Cloudflare proxy → cloudflared tunnel → NAS.

### 2.2 Cloudflare Access (autentizace)

Vault endpointy **nejsou** chráněny Cloudflare Access service tokenem jako `llm.ol1n.com` — vault má vlastní autentizaci (API klíč v headeru nebo JWT). Cloudflare Access aplikuj jen na admin/debug endpointy pokud je přidáš.

**Výjimka:** webhook endpoint `/upload/payment-confirm` musí být veřejný (NOWPayments ho volá bez autentizace). Ujisti se, že Cloudflare Access policy ho nevyžaduje.

### 2.3 cloudflared konfigurace

Přidej nový hostname do existující tunnel konfigurace na NAS:

```yaml
# /etc/cloudflared/config.yml — přidej do ingress pravidel
tunnel: <tvůj-tunnel-id>
credentials-file: /etc/cloudflared/<tunnel-id>.json

ingress:
  # ... existující pravidla (llm.ol1n.com atd.) ...

  - hostname: super.ol1n.com
    service: http://localhost:2020   # Caddy naslouchá na 2020 pro vault

  - service: http_status:404         # fallback musí být poslední
```

Pokud máš Caddy už na portu 80/443 pro jiné služby, použij **named virtual host** na separátním portu (2020) nebo přidej super.ol1n.com přímo do existujícího Caddyfile.

### 2.4 Caddy konfigurace

```caddyfile
# /etc/caddy/Caddyfile — přidej blok pro vault

super.ol1n.com {
    # Cloudflare Tunnel zajišťuje TLS termination — Caddy zde neřeší certifikát
    # Pokud tunnel předává plain HTTP na localhost, použij:
    # bind 127.0.0.1

    # Statické soubory (stažené artefakty s TTL)
    handle /files/* {
        root * /data/vault/files
        file_server
    }

    # Webhook NOWPayments — musí být bez rate limitingu
    handle /upload/payment-confirm {
        reverse_proxy localhost:8090
    }

    # Veškerý ostatní provoz na Go backend
    handle {
        reverse_proxy localhost:8090 {
            # SSE keepalive pro stream statusu
            flush_interval -1
        }
    }

    # Základní rate limiting (vyžaduje Caddy L4 nebo caddy-ratelimit plugin)
    # Alternativně použij Cloudflare Rate Limiting rules v dashboardu
}
```

**Caddy verze:** doporučuji `caddy 2.8+` pro nativní `flush_interval -1` (SSE podpora). Na NAS Ubuntu:

```bash
apt install -y debian-keyring debian-archive-keyring apt-transport-https
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
echo "deb [signed-by=/usr/share/keyrings/caddy-stable-archive-keyring.gpg] https://dl.cloudsmith.io/public/caddy/stable/deb/debian any-release main" > /etc/apt/sources.list.d/caddy-stable.list
apt update && apt install caddy
```

### 2.5 Caddy ↔ cloudflared pořadí

```
Internet → Cloudflare Edge → cloudflared (NAS) → Caddy :2020 → Go :8090
```

cloudflared poslouchá a přeposílá na Caddy. Caddy dělá routing a přeposílá na Go. Go backend nikdy není vystaven přímo na internet.

---

## 3. Go backend — struktura projektu

```
ol1n-vault/
├── cmd/
│   └── vault/
│       └── main.go              # entry point, DI, HTTP server
├── internal/
│   ├── config/
│   │   └── config.go            # env vars, validace
│   ├── db/
│   │   ├── db.go                # SQLite init, migrace
│   │   └── queries.go           # CRUD operace
│   ├── handler/
│   │   ├── upload.go            # POST /upload/init, POST /upload/payment-confirm
│   │   ├── download.go          # POST /download/{id}, GET /download/{id}/status
│   │   └── health.go            # GET /health
│   ├── nntp/
│   │   ├── client.go            # NNTP spojení, upload, download
│   │   ├── yenc.go              # yEnc enkódování/dekódování
│   │   └── nzb.go               # NZB generování a parsování
│   ├── payment/
│   │   ├── nowpayments.go       # NOWPayments API klient
│   │   └── webhook.go           # HMAC-SHA512 verifikace
│   ├── jobs/
│   │   ├── worker.go            # goroutine pool, job queue
│   │   ├── upload_job.go        # upload pipeline
│   │   └── download_job.go      # download pipeline
│   ├── storage/
│   │   └── files.go             # správa /data/vault/files, TTL čištění
│   └── blockchain/
│       └── polygon.go           # volitelná notarizace (go-ethereum)
├── migrations/
│   ├── 001_init.sql
│   └── 002_jobs.sql
├── go.mod
├── go.sum
├── Makefile
└── Dockerfile
```

### Závislosti (go.mod)

```go
module github.com/lioilsources/ol1n-vault

go 1.23

require (
    github.com/go-chi/chi/v5        v5.1.0   // HTTP router
    github.com/mattn/go-sqlite3     v1.14.22 // SQLite driver (CGO)
    github.com/golang-migrate/migrate/v4 v4.17.0 // DB migrace
    // Volitelné:
    github.com/ethereum/go-ethereum v1.14.0  // Polygon notarizace
)
```

> **CGO poznámka:** `go-sqlite3` vyžaduje CGO. Na NAS ARM64 nebo x86 Ubuntu to funguje nativně. Pro cross-kompilaci z Macu použij Docker buildx nebo `GOARCH=amd64 GOOS=linux CGO_ENABLED=1 CC=x86_64-linux-gnu-gcc go build`.

### Alternativa bez CGO

Pokud chceš build bez CGO, nahraď `go-sqlite3` za `modernc.org/sqlite` — čistý Go port SQLite, o ~15% pomalejší ale bez závislosti na C kompilátoru.

### Environment variables

```bash
# /etc/ol1n-vault/env

VAULT_PORT=8090
VAULT_DB_PATH=/data/vault/vault.db
VAULT_NZB_DIR=/data/vault/nzb
VAULT_FILES_DIR=/data/vault/files
VAULT_FILES_TTL_HOURS=24

# Eweka UseNet
NNTP_HOST=news.eweka.nl
NNTP_PORT=563
NNTP_USER=tvuj_login
NNTP_PASS=tvoje_heslo
NNTP_CONNECTIONS=8          # souběžná spojení pro upload/download
NNTP_NEWSGROUP=alt.binaries.test  # tvůj newsgroup pro upload

# NOWPayments
NOWPAYMENTS_API_KEY=...
NOWPAYMENTS_IPN_SECRET=...
NOWPAYMENTS_SUCCESS_URL=https://super.ol1n.com/upload/done
NOWPAYMENTS_CANCEL_URL=https://super.ol1n.com/upload/cancel

# Cena uploadu (USD)
UPLOAD_PRICE_USD=0.50

# Polygon (volitelné)
POLYGON_RPC_URL=https://polygon-rpc.com
POLYGON_PRIVATE_KEY=...     # peněženka pro platbu gas fees
```

---

## 4. SQLite schéma

```sql
-- migrations/001_init.sql

CREATE TABLE artifacts (
    id              TEXT PRIMARY KEY,   -- SHA256(NZB obsah) jako hex
    filename        TEXT NOT NULL,
    content_type    TEXT NOT NULL,      -- MIME typ detekovaný při uploadu
    size_bytes      INTEGER NOT NULL,
    nzb_path        TEXT,               -- relativní cesta v VAULT_NZB_DIR
    status          TEXT NOT NULL DEFAULT 'pending_payment',
    -- stavy: pending_payment → uploading → ready → notarized | failed
    payment_id      TEXT,               -- NOWPayments payment_id
    payment_status  TEXT,               -- finished | failed
    tx_hash         TEXT,               -- Polygon tx hash (NULL = nenotarizováno)
    created_at      INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at      INTEGER NOT NULL DEFAULT (unixepoch())
);

CREATE TABLE jobs (
    id              TEXT PRIMARY KEY,   -- UUID v4
    artifact_id     TEXT NOT NULL REFERENCES artifacts(id),
    type            TEXT NOT NULL,      -- upload | download
    status          TEXT NOT NULL DEFAULT 'queued',
    -- stavy: queued → running → done | failed
    progress        INTEGER DEFAULT 0,  -- 0-100
    error_msg       TEXT,
    result_url      TEXT,               -- výsledná URL po dokončení download jobu
    created_at      INTEGER NOT NULL DEFAULT (unixepoch()),
    updated_at      INTEGER NOT NULL DEFAULT (unixepoch())
);

CREATE TABLE download_tokens (
    token           TEXT PRIMARY KEY,   -- náhodný 32-byte hex token
    artifact_id     TEXT NOT NULL REFERENCES artifacts(id),
    expires_at      INTEGER NOT NULL,   -- unixepoch() + TTL
    created_at      INTEGER NOT NULL DEFAULT (unixepoch())
);

CREATE INDEX idx_artifacts_status ON artifacts(status);
CREATE INDEX idx_jobs_artifact    ON jobs(artifact_id);
CREATE INDEX idx_tokens_expires   ON download_tokens(expires_at);
```

---

## 5. Upload pipeline

### HTTP endpointy

```
POST /upload/init
    Body: multipart/form-data { file, filename }
    Response: { job_id, payment_url, payment_id, expires_at }

POST /upload/payment-confirm   (NOWPayments webhook)
    Header: x-nowpayments-sig: <HMAC-SHA512>
    Body: JSON payload
    Response: 200 OK

GET /upload/status/{job_id}    (SSE stream)
    Response: text/event-stream
    Events: { status, progress, artifact_id? }
```

### Sekvenční flow

```
1. Flutter:  POST /upload/init  →  backend přijme soubor do temp
2. Backend:  detekuj MIME, spočítej size, vygeneruj job_id (UUID)
3. Backend:  ulož soubor do /data/vault/tmp/{job_id}
4. Backend:  zavolej NOWPayments API → vytvoř invoice
5. Backend:  ulož artifact (status=pending_payment) + job do SQLite
6. Backend:  vrať { job_id, payment_url } Flutter klientovi
7. Flutter:  otevři payment_url (webview nebo external browser)
7. Flutter:  otevři SSE stream GET /upload/status/{job_id}

8. NOWPayments: POST /upload/payment-confirm  (webhook)
9. Backend:  ověř HMAC-SHA512 signaturu
10. Backend: UPDATE artifacts SET status='uploading' WHERE status='pending_payment'
11. Backend: spusť goroutine: uploadJob(job_id)

uploadJob goroutine:
12. Načti soubor z /data/vault/tmp/{job_id}
13. yEnc enkóduj po částech (750KB segmenty = standard UseNet)
14. Pro každý segment: NNTP POST do Eweka newsgroup
15. Generuj NZB XML ze získaných message-ID
16. Ulož NZB do /data/vault/nzb/{artifact_id}.nzb
17. SHA256(NZB obsah) → artifact_id
18. UPDATE artifacts: status=ready, nzb_path=..., id=artifact_id
19. Smaž temp soubor
20. Push SSE event: { status: "ready", artifact_id }
```

### yEnc segmentace

Standardní UseNet segment = **750 000 bytů** před enkódováním. Go implementace:

```go
const SegmentSize = 750_000  // bytes

func (c *Client) UploadFile(ctx context.Context, filename string, data []byte, newsgroup string) ([]MessageID, error) {
    segments := splitSegments(data, SegmentSize)
    ids := make([]MessageID, 0, len(segments))
    
    for i, seg := range segments {
        encoded := yEncEncode(seg, filename, i+1, len(segments))
        msgID, err := c.postArticle(ctx, newsgroup, filename, encoded)
        if err != nil {
            return ids, fmt.Errorf("segment %d/%d failed: %w", i+1, len(segments), err)
        }
        ids = append(ids, msgID)
    }
    return ids, nil
}
```

---

## 6. Download pipeline

### HTTP endpointy

```
POST /download/{artifact_id}
    Response: { job_id }

GET /download/{job_id}/status  (SSE stream)
    Events: { status, progress, download_url? }

GET /files/{token}/{filename}
    Response: binary file stream (Caddy file_server)
```

### Sekvenční flow

```
1. Flutter:  POST /download/{artifact_id}
2. Backend:  ověř že artifact existuje a má status=ready
3. Backend:  vygeneruj job_id, ulož job do SQLite
4. Backend:  spusť goroutine: downloadJob(job_id, artifact_id)
5. Backend:  vrať { job_id } okamžitě
6. Flutter:  otevři SSE GET /download/{job_id}/status

downloadJob goroutine:
7.  Načti nzb_path z SQLite
8.  Parsuj NZB XML → list (newsgroup, message-ID) pro každý segment
9.  Pro každý segment: NNTP BODY <message-id>
10. yDecode každý segment
11. Složi segmenty v pořadí → výsledný soubor
12. SHA256(soubor) — volitelná kontrola integrity
13. Ulož do /data/vault/files/{artifact_id}/
14. Vygeneruj download token (32 random bytes, TTL 24h)
15. Ulož token do SQLite tabulky download_tokens
16. Sestav result_url: https://super.ol1n.com/files/{token}/{filename}
17. UPDATE job: status=done, result_url=...
18. Push SSE: { status: "done", download_url: "...", content_type: "..." }
```

### Větvení podle content_type

```go
func buildResultURL(token, filename, contentType string) ResultPayload {
    baseURL := fmt.Sprintf("https://super.ol1n.com/files/%s/%s", token, filename)
    
    switch {
    case strings.HasPrefix(contentType, "video/"):
        return ResultPayload{Type: "download", URL: baseURL}
    
    case strings.HasPrefix(contentType, "image/") && strings.HasSuffix(filename, ".zip"):
        // Rozbal archiv, vrať gallery endpoint
        return ResultPayload{Type: "gallery", URL: baseURL + "/gallery"}
    
    case strings.HasPrefix(contentType, "image/"):
        return ResultPayload{Type: "image", URL: baseURL}
    
    default:
        return ResultPayload{Type: "download", URL: baseURL}
    }
}
```

### TTL čištění souborů

Spusť jako ticker goroutine při startu backendu:

```go
func (s *FileStore) StartCleanup(ctx context.Context, interval time.Duration) {
    ticker := time.NewTicker(interval)
    defer ticker.Stop()
    for {
        select {
        case <-ticker.C:
            s.deleteExpiredFiles(ctx)
        case <-ctx.Done():
            return
        }
    }
}
```

---

## 7. NOWPayments integrace

### 7.1 Vytvoření invoice

```go
type InvoiceRequest struct {
    PriceAmount     float64 `json:"price_amount"`
    PriceCurrency   string  `json:"price_currency"`  // "usd"
    OrderID         string  `json:"order_id"`         // job_id
    OrderDescription string `json:"order_description"`
    SuccessURL      string  `json:"success_url"`
    CancelURL       string  `json:"cancel_url"`
    IPNCallbackURL  string  `json:"ipn_callback_url"` // webhook URL
}

type InvoiceResponse struct {
    ID         string `json:"id"`
    InvoiceURL string `json:"invoice_url"`
    Status     string `json:"status"`
}

func (c *Client) CreateInvoice(ctx context.Context, jobID string, priceUSD float64) (*InvoiceResponse, error) {
    req := InvoiceRequest{
        PriceAmount:      priceUSD,
        PriceCurrency:    "usd",
        OrderID:          jobID,
        OrderDescription: "Ol1n Vault upload",
        SuccessURL:       c.cfg.SuccessURL,
        CancelURL:        c.cfg.CancelURL,
        IPNCallbackURL:   "https://super.ol1n.com/upload/payment-confirm",
    }
    // POST https://api.nowpayments.io/v1/invoice
    // Header: x-api-key: NOWPAYMENTS_API_KEY
}
```

### 7.2 Webhook verifikace

Viz sekce "NOWPayments webhook verifikace" v předchozím rozboru.

Klíčové body:
- Přečíst raw body před parserem (`io.ReadAll`)
- JSON klíče seřadit abecedně před HMAC výpočtem
- `HMAC-SHA512` s IPN Secret klíčem
- Constant-time porovnání (`hmac.Equal`)
- Idempotentní update: `WHERE status = 'pending_payment'`

### 7.3 Stavy platby

NOWPayments posílá webhook pro každý přechod stavu:

```
waiting → confirming → confirmed → finished  ← terminální úspěch
                                 → failed    ← terminální selhání
                                 → expired   ← platba nevykonána
```

Spuštění upload jobu pouze při `confirmed` nebo `finished`.

---

## 8. Blockchain notarizace (volitelná)

### Polygon OP_RETURN transakce

```go
import "github.com/ethereum/go-ethereum/ethclient"

func (n *Notarizer) NotarizeArtifact(ctx context.Context, artifactID string) (string, error) {
    // artifactID je SHA256 hash = 32 bytes
    hashBytes, _ := hex.DecodeString(artifactID)
    
    // Sestav transakci s OP_RETURN daty
    tx := types.NewTransaction(
        nonce, 
        common.Address{},   // zero address = burn
        big.NewInt(0),       // 0 ETH
        60000,               // gas limit
        gasPrice,
        hashBytes,           // data = náš hash
    )
    
    signedTx, _ := types.SignTx(tx, types.NewEIP155Signer(chainID), privateKey)
    err := client.SendTransaction(ctx, signedTx)
    return signedTx.Hash().Hex(), err
}
```

Polygon mainnet chainID = `137`. Gas fees jsou v MATIC (zlomky centu).

### Kdy spustit

Notarizace je volitelná akce kterou spustí uživatel manuálně z Flutter UI po úspěšném uploadu. Není součástí automatického upload flow.

---

## 9. Flutter integrace

### Nová záložka v SuperOl1n

```dart
// Přidej do TabBar v MainScreen
Tab(text: 'Vault'),

// VaultScreen widget
class VaultScreen extends StatefulWidget { ... }
```

### Upload flow

```dart
// 1. Výběr souboru
final result = await FilePicker.platform.pickFiles();

// 2. Init upload
final response = await vaultApi.initUpload(result.files.single);
// response: { job_id, payment_url }

// 3. Otevři platební stránku
await launchUrl(Uri.parse(response.paymentUrl));

// 4. SSE stream pro status
final stream = vaultApi.uploadStatus(response.jobId);
stream.listen((event) {
  if (event.status == 'ready') {
    // Zobraz artifact_id a možnost notarizace
  }
});
```

### SSE klient v Dart/Flutter

```dart
import 'package:http/http.dart' as http;

Stream<VaultEvent> uploadStatus(String jobId) async* {
  final request = http.Request('GET', 
    Uri.parse('https://super.ol1n.com/upload/status/$jobId'));
  final response = await http.Client().send(request);
  
  await for (final chunk in response.stream.transform(utf8.decoder)) {
    for (final line in chunk.split('\n')) {
      if (line.startsWith('data: ')) {
        final json = jsonDecode(line.substring(6));
        yield VaultEvent.fromJson(json);
      }
    }
  }
}
```

### API kontrakt

```dart
class VaultApi {
  final String baseUrl = 'https://super.ol1n.com';
  
  // Upload
  Future<InitUploadResponse> initUpload(PlatformFile file);
  Stream<VaultEvent>         uploadStatus(String jobId);
  
  // Download
  Future<DownloadJobResponse> requestDownload(String artifactId);
  Stream<VaultEvent>          downloadStatus(String jobId);
  
  // Artefakty
  Future<List<Artifact>>      listArtifacts();
  Future<Artifact>            getArtifact(String id);
  
  // Notarizace
  Future<NotarizeResponse>    notarize(String artifactId);
}
```

---

## 10. Deployment na NAS

### systemd service

```ini
# /etc/systemd/system/ol1n-vault.service

[Unit]
Description=Ol1n Vault Backend
After=network.target

[Service]
Type=simple
User=ol1n
EnvironmentFile=/etc/ol1n-vault/env
ExecStart=/usr/local/bin/ol1n-vault
Restart=on-failure
RestartSec=5s
WorkingDirectory=/data/vault

# Limity (Celeron, 4GB RAM)
MemoryMax=512M
CPUQuota=80%

[Install]
WantedBy=multi-user.target
```

### Makefile

```makefile
BINARY=ol1n-vault
BUILD_TARGET=linux/amd64   # NAS architektura

build:
	GOOS=linux GOARCH=amd64 CGO_ENABLED=1 \
	CC=x86_64-linux-gnu-gcc \
	go build -o bin/$(BINARY) ./cmd/vault

# Pokud NAS je ARM64:
build-arm64:
	GOOS=linux GOARCH=arm64 CGO_ENABLED=1 \
	CC=aarch64-linux-gnu-gcc \
	go build -o bin/$(BINARY)-arm64 ./cmd/vault

deploy:
	scp bin/$(BINARY) nas:/usr/local/bin/$(BINARY)
	ssh nas "systemctl restart ol1n-vault"

logs:
	ssh nas "journalctl -u ol1n-vault -f"
```

### Adresářová struktura na NAS

```bash
mkdir -p /data/vault/{nzb,files,tmp}
chown -R ol1n:ol1n /data/vault
chmod 750 /data/vault
```

---

## 11. Doporučení pro Claude Code

### Model

Používej **claude-opus-4-5** pro:
- Architektonická rozhodnutí a design review
- Komplexní debugging přes více souborů
- Psaní NNTP/yEnc logiky (edge cases)

Používej **claude-sonnet-4-5** (výchozí) pro:
- Generování boilerplate kódu
- SQLite migrace a CRUD
- HTTP handlery
- Unit testy

### Efficiency doporučení

1. **Jeden soubor = jeden kontext.** Neotevírej celý projekt najednou. Pracuj vždy s konkrétním `internal/nntp/client.go` nebo `internal/payment/webhook.go`.

2. **Pořadí implementace:**
   ```
   1. db/ — migrace a queries (základ pro vše ostatní)
   2. config/ — načítání env vars
   3. payment/webhook.go — kritická část, chce review
   4. nntp/client.go — NNTP spojení a základní POST
   5. nntp/yenc.go — enkódování
   6. nntp/nzb.go — generování NZB
   7. jobs/worker.go — goroutine pool
   8. jobs/upload_job.go
   9. handler/upload.go
   10. jobs/download_job.go
   11. handler/download.go
   12. storage/files.go — TTL čištění
   13. blockchain/polygon.go — až vše ostatní funguje
   ```

3. **Použij tento soubor jako kontext.** Na začátku každé Claude Code session přilož `OL1N_VAULT_PLAN.md` jako kontext: `claude --context OL1N_VAULT_PLAN.md`.

4. **NNTP knihovna.** Neimplementuj NNTP protokol od nuly — použij existující Go knihovnu jako základ:
   - `github.com/mnightingale/go-nntp` — jednoduchá, aktivní
   - Nebo `net/textproto` ze stdlib pro vlastní implementaci nad TCP

5. **Test webhook lokálně** přes `cloudflared tunnel --url localhost:8090` ještě před deploymentem na NAS — NOWPayments sandbox (`api.sandbox.nowpayments.io`) posílá reálné webhooky.

### CLAUDE.md pro projekt

Vlož do kořene projektu `ol1n-vault/CLAUDE.md`:

```markdown
# ol1n-vault project context

Backend service pro Ol1n Vault — UseNet archivace s krypto platbami.

## Stack
- Go 1.23, chi router, go-sqlite3 (CGO)
- NNTP protokol → Eweka UseNet provider
- NOWPayments API pro platby
- Polygon blockchain pro notarizaci

## Deploy target
NAS Ubuntu x86_64, 4GB RAM, systemd service

## Kritická pravidla
- NOWPayments webhook: vždy číst raw body před json parserem
- JSON klíče MUSÍ být seřazeny před HMAC výpočtem
- SQLite UPDATE jobu musí být idempotentní (WHERE status='pending_payment')
- NNTP spojení: pool max 8 connections na Eweka
- Soubory v /data/vault/files/ mají TTL 24h — čistit přes ticker

## Odpovídej česky.
```

---

*Plán verze 1.0 — SuperOl1n Vault | Připraven pro Claude Code implementaci*
