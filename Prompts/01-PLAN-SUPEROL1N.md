# SuperOl1n – Implementační Plán pro Claude Code

> Verze: 1.0  
> Stack: Go (backend) + Flutter (frontend)  
> Deployment: NAS Ubuntu (Celeron) + DGX Spark (Ollama přes LAN)

---

## Přehled architektury

```
Internet
  └── superol1n.ol1n.com (Cloudflare Tunnel)
        └── Caddy reverse proxy (NAS :443 → :8090)
              └── SuperOl1n Go Backend (:8090)
                    ├── /api/ai/*      → DGX Spark LAN (192.168.x.x:11434)
                    ├── /api/sonarr/*  → localhost:8989
                    ├── /api/radarr/*  → localhost:7878
                    ├── /api/sabnzbd/* → localhost:8080
                    ├── /api/plex/*    → localhost:32400
                    └── /ws/events     → WebSocket hub

Flutter App (iOS/Android/Web)
  └── ApiClient (Dio) → superol1n.ol1n.com nebo LAN IP
```

---

## Fáze 1 – Go Backend Skeleton

### 1.1 Projekt inicializace

```bash
mkdir -p superol1n && cd superol1n
go mod init github.com/ol1ne/superol1n
```

### 1.2 Adresářová struktura

```
superol1n/
├── main.go
├── config.yaml
├── Makefile
├── internal/
│   ├── config/
│   │   └── config.go          # Načítání config.yaml (viper)
│   ├── gateway/
│   │   ├── server.go          # HTTP server setup
│   │   ├── router.go          # Registrace modulů
│   │   └── middleware/
│   │       ├── auth.go        # CF Access JWT validace
│   │       ├── cors.go        # Flutter potřebuje CORS
│   │       └── logger.go      # Structured logging (slog)
│   ├── module/
│   │   └── module.go          # Module interface
│   └── modules/
│       ├── ai/
│       │   ├── handler.go     # POST /api/ai/chat (SSE)
│       │   ├── models.go      # GET /api/ai/models
│       │   └── client.go      # Ollama LAN HTTP client
│       ├── sonarr/
│       │   ├── handler.go
│       │   └── client.go
│       ├── radarr/
│       │   ├── handler.go
│       │   └── client.go
│       ├── sabnzbd/
│       │   ├── handler.go
│       │   └── client.go
│       └── events/
│           ├── hub.go         # WebSocket hub
│           └── handler.go     # /ws/events + webhook ingress
├── db/
│   └── sqlite.go              # Chat historie (modernc.org/sqlite)
└── systemd/
    └── superol1n.service
```

### 1.3 Module interface

```go
// internal/module/module.go
package module

import "net/http"

type Module interface {
    Prefix() string        // např. "/api/ai"
    Handler() http.Handler
    Name() string
    IsHealthy() bool       // ping na downstream službu
}
```

### 1.4 config.yaml struktura

```yaml
server:
  port: 8090
  lan_only_key: "CHANGE_ME"   # API key pro LAN přístup bez CF

cloudflare:
  access_team_domain: "ol1n.cloudflareaccess.com"

ollama:
  lan_url: "http://192.168.x.x:11434"   # DGX Spark LAN IP
  default_model: "qwen3:30b-a3b"

sonarr:
  url: "http://localhost:8989"
  api_key: "SONARR_API_KEY"

radarr:
  url: "http://localhost:7878"
  api_key: "RADARR_API_KEY"

sabnzbd:
  url: "http://localhost:8080"
  api_key: "SABNZBD_API_KEY"

plex:
  url: "http://localhost:32400"
  token: "PLEX_TOKEN"

db:
  path: "/var/lib/superol1n/superol1n.db"
```

### 1.5 Auth middleware logika

```
Request přichází:
  ├── Header: X-LAN-Key → porovnej s config.lan_only_key → OK
  └── Cookie: CF_Authorization (JWT)
        └── Ověř podpis proti CF Access JWKS endpoint
              └── OK → context.WithValue(r.Context(), "user", email)
```

Závislosti:
- `github.com/spf13/viper` – config
- `github.com/golang-jwt/jwt/v5` – CF Access JWT validace
- `modernc.org/sqlite` – CGO-free SQLite
- `github.com/gorilla/websocket` – WebSocket hub

---

## Fáze 2 – AI Modul (SSE Streaming)

### 2.1 Endpoint

```
POST /api/ai/chat
Content-Type: application/json
Accept: text/event-stream

{
  "model": "qwen3:30b-a3b",
  "messages": [...],
  "conversation_id": "uuid"   // optional, pro uložení do DB
}
```

### 2.2 Handler logika

```go
func (h *Handler) Chat(w http.ResponseWriter, r *http.Request) {
    // 1. Decode request body
    // 2. Set SSE headers (Content-Type: text/event-stream, X-Accel-Buffering: no)
    // 3. POST na Ollama LAN URL /api/chat se stream:true
    // 4. Čti chunky z Ollama response body
    // 5. Každý chunk → "data: {json}\n\n" → w.Write + Flusher.Flush()
    // 6. Po dokončení ulož celou konverzaci do SQLite (pokud conversation_id)
    // 7. Pošli "data: [DONE]\n\n"
}
```

### 2.3 Models endpoint

```
GET /api/ai/models
→ Forwarduje GET http://192.168.x.x:11434/api/tags
→ Vrátí list modelů pro Flutter model picker
```

---

## Fáze 3 – Media Moduly

### 3.1 Sonarr endpoints (příklady)

```
GET  /api/sonarr/series              → Sonarr GET /api/v3/series
GET  /api/sonarr/series/lookup?q=X   → Sonarr GET /api/v3/series/lookup
POST /api/sonarr/series              → Sonarr POST /api/v3/series (přidat seriál)
GET  /api/sonarr/calendar            → Sonarr GET /api/v3/calendar
GET  /api/sonarr/queue               → Sonarr GET /api/v3/queue
```

Implementace: jednoduchý reverse proxy handler – přepiš prefix, přidej `X-Api-Key` header.

### 3.2 Radarr endpoints (stejný pattern)

```
GET  /api/radarr/movie
GET  /api/radarr/movie/lookup?q=X
POST /api/radarr/movie
GET  /api/radarr/queue
```

### 3.3 SABnzbd endpoints

```
GET /api/sabnzbd/queue    → SABnzbd ?mode=queue&output=json
GET /api/sabnzbd/history  → SABnzbd ?mode=history&output=json
POST /api/sabnzbd/pause   → SABnzbd ?mode=pause
POST /api/sabnzbd/resume  → SABnzbd ?mode=resume
```

---

## Fáze 4 – Events (WebSocket + Webhooks)

### 4.1 WebSocket Hub

```go
// Flutter se připojí na ws://superol1n.ol1n.com/ws/events
// Hub broadcastuje všem připojeným klientům

type Event struct {
    Type    string          `json:"type"`    // "sonarr.download", "radarr.grab", "sabnzbd.complete"
    Payload json.RawMessage `json:"payload"`
    Time    time.Time       `json:"time"`
}
```

### 4.2 Webhook ingress

```
POST /webhooks/sonarr   → parsuj Sonarr Connect payload → broadcast Event
POST /webhooks/radarr   → parsuj Radarr Connect payload → broadcast Event
```

V Sonarr/Radarr nastav Webhook Connection na `http://localhost:8090/webhooks/sonarr`.

---

## Fáze 5 – Flutter App

### 5.1 Závislosti (pubspec.yaml)

```yaml
dependencies:
  flutter:
    sdk: flutter
  dio: ^5.x                    # HTTP client
  go_router: ^14.x             # Routing
  provider: ^6.x               # State management (nebo riverpod)
  web_socket_channel: ^3.x     # WebSocket
  shared_preferences: ^2.x     # Lokální nastavení (base URL, etc.)
  cached_network_image: ^3.x   # Plex artwork
  flutter_markdown: ^0.7.x     # LLM odpovědi v markdown
```

### 5.2 Adresářová struktura

```
lib/
├── main.dart
├── core/
│   ├── api_client.dart        # Dio instance, base URL, auth interceptor
│   ├── config.dart            # Base URL (LAN vs external), nastavení
│   ├── router.dart            # go_router + bottom nav tab state
│   └── websocket_service.dart # Připojení na /ws/events
├── modules/
│   ├── ai/
│   │   ├── ai_screen.dart         # Tab obrazovka
│   │   ├── chat_view.dart         # ListView zpráv
│   │   ├── message_input.dart     # TextField + send button
│   │   ├── model_picker.dart      # DropdownButton s modely
│   │   ├── message_bubble.dart    # Widget pro jednu zprávu (SSE aware)
│   │   └── ai_provider.dart       # State: konverzace, streaming stav
│   ├── media/
│   │   ├── media_screen.dart      # Tab s Sonarr+Radarr taby
│   │   ├── series_search.dart
│   │   ├── movie_search.dart
│   │   └── media_provider.dart
│   ├── downloads/
│   │   ├── downloads_screen.dart  # SABnzbd queue
│   │   └── downloads_provider.dart
│   └── events/
│       ├── events_screen.dart     # Feed notifikací
│       └── events_provider.dart   # WebSocket listener
└── shared/
    ├── theme/
    │   └── app_theme.dart         # Dark theme, barvy, fonty
    └── widgets/
        ├── loading_indicator.dart
        └── error_view.dart
```

### 5.3 Bottom Navigation

```dart
// 4 taby, persistent state (IndexedStack)
NavigationBar(
  destinations: [
    NavigationDestination(icon: Icon(Icons.smart_toy), label: 'AI'),
    NavigationDestination(icon: Icon(Icons.movie), label: 'Media'),
    NavigationDestination(icon: Icon(Icons.download), label: 'Downloads'),
    NavigationDestination(icon: Icon(Icons.notifications), label: 'Events'),
  ],
)
```

### 5.4 SSE Streaming v Dart

```dart
// Dio ResponseType.stream → poslouchej chunky
final response = await _dio.post(
  '/api/ai/chat',
  data: payload,
  options: Options(responseType: ResponseType.stream),
);

response.data.stream
  .transform(utf8.decoder)
  .transform(const LineSplitter())
  .listen((line) {
    if (line.startsWith('data: ') && line != 'data: [DONE]') {
      final json = jsonDecode(line.substring(6));
      // append token do message bubble
    }
  });
```

### 5.5 Konfigurace Base URL

Settings screen (gear icon v AppBar) kde uživatel zadá:
- `http://192.168.x.x:8090` pro LAN
- `https://superol1n.ol1n.com` pro external

Uloží do `shared_preferences`. ApiClient si načte při startu.

---

## Fáze 6 – Deployment

### 6.1 Makefile (cross-compile na NAS)

```makefile
BINARY = superol1n
NAS_HOST = nas.local
NAS_PATH = /usr/local/bin/superol1n

build-nas:
	GOOS=linux GOARCH=amd64 go build -o $(BINARY) ./cmd/server

deploy:
	make build-nas
	scp $(BINARY) $(NAS_HOST):$(NAS_PATH)
	ssh $(NAS_HOST) "sudo systemctl restart superol1n"
```

### 6.2 systemd service

```ini
[Unit]
Description=SuperOl1n Backend
After=network.target

[Service]
ExecStart=/usr/local/bin/superol1n
EnvironmentFile=/etc/superol1n/env
WorkingDirectory=/var/lib/superol1n
Restart=always
RestartSec=5
User=superol1n

[Install]
WantedBy=multi-user.target
```

### 6.3 Caddy konfigurace (přidat do existujícího Caddyfile)

```caddyfile
superol1n.ol1n.com {
    reverse_proxy localhost:8090
}
```

### 6.4 Cloudflare Tunnel

```bash
# Přidat do existující tunnel config
# config.yml na NAS kde běží cloudflared

ingress:
  - hostname: superol1n.ol1n.com
    service: http://localhost:8090
  # ... existující pravidla ...
```

CF Access Policy: stejná jako ostatní služby (Google OAuth).

---

## Pořadí implementace (doporučené)

```
[x] Fáze 1  Go skeleton + config + auth middleware
[x] Fáze 2  AI modul + Ollama SSE proxy
[x] Fáze 5a Flutter projekt + ApiClient + AI tab (SSE chat)
            ↑ Tady máš MVP – fungující AI chat

[x] Fáze 3a Sonarr modul
[x] Fáze 3b Radarr modul
[x] Fáze 5b Flutter Media tab

[x] Fáze 3c SABnzbd modul
[x] Fáze 5c Flutter Downloads tab

[x] Fáze 4  WebSocket hub + webhooks
[x] Fáze 5d Flutter Events tab

[x] Fáze 6  Deployment + Caddy + CF Tunnel
```

---

## Poznámky pro Claude Code

- Go verze: **1.23+** (range over integers, slog stdlib)
- Nepoužívej CGO – `modernc.org/sqlite` je pure Go
- Error handling: vždy `fmt.Errorf("context: %w", err)`
- HTTP handlery: `net/http` stdlib, žádný framework
- Logování: `log/slog` stdlib
- Ollama LAN IP bude ve `config.yaml` – nikdy hardcode
- Flutter: null safety, Dart 3.x
- Všechny API klíče pouze v `config.yaml` / env, nikdy v kódu

---

*Tento plán začni od Fáze 1 a postupuj sekvenčně.*
