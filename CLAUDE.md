# Ol1nSuperApp — CLAUDE.md

## Overview

Flutter super-app (`superol1n_app`) with a Go backend (`superol1n`). App aggregates multiple features under one shell. Backend is a Go HTTP service with SQLite, deployed via systemd.

Flutter app lives in `superol1n_app/`, Go backend in `superol1n/`.

## Flutter App

```bash
cd superol1n_app
flutter pub get
flutter run
flutter run -d ios
flutter run -d android
flutter build apk
flutter build ios
flutter analyze
```

```
superol1n_app/lib/
├── main.dart
└── modules/             # Feature modules (one dir per feature)
    shared/              # Shared widgets, utilities
    core/
```

## Go Backend

```bash
cd superol1n
go build -o bin/superol1n ./cmd/...
make build

# Run
./bin/superol1n

# Config
cp config.yaml.example config.yaml
# edit config.yaml
```

```
superol1n/
├── cmd/                 # Entry point
├── config.yaml          # App config (git-ignored)
├── config.yaml.example  # Config template
├── db/                  # SQLite schema, migrations
├── internal/            # Business logic
├── go.mod
├── Makefile
└── systemd/             # systemd unit file for NAS deploy
```

## Vault

`ol1n-vault/` — separate vault/secrets service or key store used by the app.

## Conventions

- Backend port: check `config.yaml.example`
- SQLite DB path: configured in `config.yaml`
- Systemd service: in `superol1n/systemd/`
