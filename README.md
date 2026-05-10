# outbound-api

> Automated algorithmic stock-trading service — real-time signals, ATR-based risk sizing, and broker execution via Alpaca.

[![Go](https://img.shields.io/badge/go-1.26-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev)
[![CI](https://github.com/wu-piyaphon/outbound-api/actions/workflows/deploy.yml/badge.svg)](https://github.com/wu-piyaphon/outbound-api/actions/workflows/deploy.yml)
[![Docker](https://img.shields.io/badge/image-ghcr.io-2496ED?style=flat-square&logo=docker&logoColor=white)](https://github.com/wu-piyaphon/outbound-api/pkgs/container/outbound-api)
[![License](https://img.shields.io/badge/license-MIT-green?style=flat-square)](LICENSE)

<br />

![Banner](assets/banner.png)

<br />

---

## Overview

**outbound-api** is a production-deployed, rules-based US equities trading bot written in Go. It streams real-time IEX bar data from [Alpaca](https://alpaca.markets) for a configurable watchlist, evaluates every bar through a four-layer signal pipeline (trend, momentum, sentiment, and risk sizing), and autonomously places buy and sell orders when all conditions align.

The service is live on a Vultr VPS, deployed continuously via GitHub Actions. It is designed to run 24/7 with automatic reconnection, graceful shutdown, and rollback on failed deploys.

---

## Features

### Signal Engine
- **4-layer signal evaluation** — trend (EMA-200), momentum (RSI-14 &lt; 35), news sentiment, and ATR-based position sizing applied in sequence before any order is placed
- **Indicator cache seeded from 14 months of historical daily bars** at startup — zero broker REST calls in the hot evaluation path
- **News sentiment TTL cache** (5 min per symbol) to avoid redundant API round-trips on every bar tick
- **Daily indicator re-seed** so EMA, RSI, and ATR values incorporate each new day's bar without restarting the service

### Execution
- **Atomic bot FSM** (`running` / `paused` / `stopped`) — state transitions are safe across concurrent workers
- **Pre-insert trade record before placing the broker order** so a crash between the two leaves an auditable pending row, not a ghost position
- **Idempotent exit handling** — duplicate stop-loss/take-profit triggers are absorbed via a unique-constraint check; no double-sells
- **Real-time trade update stream** reconciles fill prices, quantities, and fees from Alpaca back into the database as events arrive

### Infrastructure
- **Buffered worker pool** (5 goroutines, channel depth 200) with non-blocking sends — bars are dropped with a warning log rather than blocking the stream under backpressure
- **Exponential-backoff reconnect** (1 s → 64 s) for both the bar stream and trade-update stream, supervised until context cancellation
- **Embedded SQL migrations** run automatically at startup via `golang-migrate` — no manual steps needed after deploy
- **Timing-safe API key auth** via `crypto/subtle.ConstantTimeCompare` on all bot-control routes

---

## Architecture

![Architecture](assets/architecture.png)

```mermaid
flowchart TD
    PG[(PostgreSQL\nSupabase)] -->|"active watchlist symbols"| WatchlistSvc

    WatchlistSvc -->|"subscribe / unsubscribe\nevery 30 s"| BarStream

    subgraph alpaca [Alpaca]
        BarStream["IEX Bar Stream\n(WebSocket)"]
        TradeUpdateStream["Trade Update Stream\n(WebSocket)"]
        AlpacaREST["Alpaca REST API"]
        AlpacaNews["Alpaca News"]
    end

    BarStream -->|"bar events"| barChan["barChan\n(buffered 200)"]
    barChan -->|"5 concurrent workers"| BotWorker

    BotWorker --> ExitCheck["Exit Check\n(stop-loss / take-profit)"]
    BotWorker --> SignalEval["Signal Evaluation"]

    ExitCheck -->|"exit condition met"| TradeService
    SignalEval -->|"Layer 1: price > EMA-200\nLayer 2: RSI < 35\nLayer 3: sentiment score > 0"| SignalService
    SignalService -->|"buy signal"| TradeService

    AlpacaNews -->|"keyword scoring"| SentimentCache["Sentiment Cache\n(TTL 5 min)"]
    SentimentCache --> SignalEval

    TradeService -->|"place / close order"| AlpacaREST
    TradeUpdateStream -->|"fill / cancel events"| TradeService

    TradeService -->|"persist"| PG
    SignalService -->|"persist"| PG

    subgraph indicatorCache [Indicator Cache]
        EMA200["EMA-200"]
        RSI14["RSI-14"]
        ATR14["ATR-14"]
    end

    PG -.->|"14 months of daily bars\nat startup + daily re-seed"| indicatorCache
    indicatorCache --> SignalEval
    indicatorCache --> TradeService
```

### Component Summary

| Component | Role |
|---|---|
| `cmd/server/main.go` | Wire-up: config, DB, repos, services, streams, HTTP, graceful shutdown |
| `internal/bot` | Atomic FSM — `running` / `paused` / `stopped` |
| `internal/indicator` | EMA / RSI / ATR math + thread-safe in-memory cache |
| `internal/sentiment` | Alpaca News keyword scoring + TTL-cached provider |
| `internal/service` | Business logic: signal evaluation, trade execution, account budgets |
| `internal/repository` | DB queries per domain entity; shared transaction context |
| `internal/alpaca` | Broker REST client, bar stream, trade-update stream, event mapper |
| `internal/database` | pgxpool setup, `golang-migrate` runner, `Transactor` helper |
| `internal/http` | Bot-control handlers + `RequireAPIKey` middleware |
| `migrations/` | Numbered up/down SQL files embedded via `embed.FS` |

---

## Tech Stack

| Category | Technology |
|---|---|
| Language | Go 1.26 |
| HTTP | `net/http` + `ServeMux` (stdlib, no framework) |
| Database | PostgreSQL via `jackc/pgx/v5` (connection pool) |
| Migrations | `golang-migrate/migrate/v4` with embedded SQL |
| Broker / Market Data | `alpacahq/alpaca-trade-api-go/v3` — REST + WebSocket |
| Decimal arithmetic | `shopspring/decimal` (no floating-point rounding) |
| Logging | `log/slog` (structured JSON to stdout) |
| Sentiment | Alpaca News API + keyword scoring + TTL cache |
| Container | Docker multi-stage build → distroless `scratch` image |
| Registry | GitHub Container Registry (GHCR) |
| CI/CD | GitHub Actions → GHCR → Vultr VPS (SSH + Docker Compose) |
| Hosting | Vultr VPS running Docker Compose |

---

## Signal Logic

![Signal Layers](assets/signal-layers.png)

A buy signal is only generated when **all four layers** pass on the same bar. Any layer failing short-circuits the evaluation immediately.

| Layer | Condition | Indicator | Rationale |
|---|---|---|---|
| **1 — Trend** | Close price > EMA-200 (daily) | EMA-200 | Only buy in a confirmed long-term uptrend |
| **2 — Momentum** | RSI-14 (daily) &lt; 35 | RSI-14 | Enter on an oversold pullback within the trend |
| **3 — Sentiment** | Alpaca News keyword score > 0 | News (TTL-cached) | Avoid entries during negative news cycles |
| **4 — Position sizing** | Available budget > minimum notional | ATR-14 | Size the trade so that a 1-ATR adverse move equals the risk budget |

**Position sizing formula:**

```
stop_distance = ATR × ATR_RISK_MULTIPLIER          # default 2.0
quantity      = (budget × RISK_PER_TRADE_PCT) / stop_distance
take_profit   = entry_price + ATR × TAKE_PROFIT_MULTIPLIER  # default 3.0
```

Exit orders (stop-loss and take-profit) are placed as limit/stop orders immediately after the entry fill is confirmed via the trade-update stream.

---

## API Reference

All bot-control endpoints require an `Authorization: Bearer <BOT_API_KEY>` header.

| Method | Path | Auth | Response | Description |
|---|---|---|---|---|
| `GET` | `/health` | None | `200 OK` / `503` | DB liveness check — used by the deploy pipeline |
| `POST` | `/bot/start` | Bearer | `200 OK` | Transition bot to `running`; begins processing bars |
| `POST` | `/bot/pause` | Bearer | `200 OK` | Transition bot to `paused`; stream stays connected, no new signals |
| `POST` | `/bot/stop` | Bearer | `200 OK` | Transition bot to `stopped` |
| `GET` | `/bot/status` | Bearer | `{"status":"running"}` | Return current FSM state |

---

## Database Schema

Four tables, all using `UUID` primary keys and `TIMESTAMPTZ` timestamps.

```
watchlists
├── symbol        VARCHAR(10) PK
├── is_active     BOOLEAN
└── created_at    TIMESTAMPTZ

signals
├── id                UUID PK
├── symbol            VARCHAR(10)
├── side              side (buy | sell)
├── price_at_signal   DECIMAL(19,4)
├── indicators        JSONB          — EMA, RSI, ATR snapshot at signal time
├── is_executed       BOOLEAN
├── reasoning         TEXT
└── created_at        TIMESTAMPTZ

account_transfers
├── id                UUID PK
├── type              transfer_type
├── amount_thb        DECIMAL(19,4)  — source currency (THB)
├── amount_usd        DECIMAL(19,4)  — converted USD budget
├── fee_thb / fee_usd DECIMAL(19,4)
├── exchange_rate     DECIMAL(19,4)
├── target_trades     INTEGER        — total trade slots funded
├── remaining_trades  INTEGER        — decremented on each buy
└── created_at / updated_at

trades
├── id                UUID PK
├── parent_id         UUID → trades(id)   — exit trade links back to entry
├── signal_id         UUID → signals(id)
├── alpaca_order_id   VARCHAR(50) UNIQUE
├── symbol            VARCHAR(10)
├── side              side
├── quantity          DECIMAL(19,4)
├── price_per_unit    DECIMAL(19,4)   — requested price
├── avg_fill_price    DECIMAL(19,4)   — filled price from broker
├── commission_fee    DECIMAL(19,4)
├── fx_fee_amortized  DECIMAL(19,4)
├── status            order_status
├── metadata          JSONB
└── filled_at / created_at
```

**Relationships:** an exit `trade` references its entry via `parent_id`; both reference the originating `signal`. Budget is tracked through `account_transfers.remaining_trades`.

---

## Getting Started

### Prerequisites

- Go 1.26+
- PostgreSQL (or a [Supabase](https://supabase.com) project)
- [Alpaca paper trading account](https://app.alpaca.markets) (free)

### Local setup

```bash
# 1. Clone
git clone https://github.com/wu-piyaphon/outbound-api.git
cd outbound-api

# 2. Configure
cp .env.example .env
# Edit .env — fill in ALPACA_*, DATABASE_URL, BOT_API_KEY

# 3. Run (migrations apply automatically on startup)
go run ./cmd/server

# 4. Verify
curl http://localhost:8080/health
```

### Using Docker

```bash
docker build -t outbound-api .
docker run --env-file .env -p 8080:8080 outbound-api
```

### Controlling the bot

```bash
BOT_API_KEY=your_key

# Start trading
curl -X POST http://localhost:8080/bot/start \
  -H "Authorization: Bearer $BOT_API_KEY"

# Check state
curl http://localhost:8080/bot/status \
  -H "Authorization: Bearer $BOT_API_KEY"

# Pause (stream stays connected, no new signals)
curl -X POST http://localhost:8080/bot/pause \
  -H "Authorization: Bearer $BOT_API_KEY"
```

---

## Configuration

Copy `.env.example` to `.env` and populate the values below.

| Variable | Required | Default | Description |
|---|---|---|---|
| `ALPACA_API_KEY` | Yes | — | Alpaca API key ID |
| `ALPACA_API_SECRET` | Yes | — | Alpaca API secret key |
| `ALPACA_BASE_URL` | Yes | — | Broker endpoint — paper: `https://paper-api.alpaca.markets`, live: `https://api.alpaca.markets` |
| `DATABASE_URL` | Yes | — | PostgreSQL connection string (`postgres://user:pass@host/db`) |
| `BOT_API_KEY` | Yes | — | Secret used to authenticate all `/bot/*` control requests |
| `PORT` | No | `8080` | HTTP port the server listens on |
| `BOT_AUTOSTART` | No | `true` | Set to `false` to start the bot paused; useful for staged rollouts |
| `RISK_PER_TRADE_PCT` | No | `0.01` | Fraction of available budget risked per trade (e.g. `0.01` = 1%) |
| `ATR_RISK_MULTIPLIER` | No | `2.0` | ATR multiplier for stop-loss distance and position sizing |
| `TAKE_PROFIT_MULTIPLIER` | No | `3.0` | ATR multiplier for take-profit level (gives a 1.5 : 1 reward/risk at default) |
| `COMMISSION_FEE_PCT` | No | `0.0005` | Fractional commission deducted from notional per filled trade |
| `FX_FEE_PCT` | No | `0.0001` | Fractional FX conversion fee amortised per filled trade |

---

## Deployment

The deploy pipeline lives in [`.github/workflows/deploy.yml`](.github/workflows/deploy.yml) and runs automatically on every push to `main`.

```
push to main
  └── Job: test
        go test ./...
  └── Job: deploy (needs: test)
        1. Login to GHCR
        2. docker build-push → ghcr.io/wu-piyaphon/outbound-api:latest
                              ghcr.io/wu-piyaphon/outbound-api:<sha>
        3. SSH into Vultr VPS
           a. docker compose pull outbound-api
           b. docker compose up -d outbound-api
           c. Health loop: curl /health × 10 (3 s apart)
           d. On failure → re-tag previous <sha> as latest → restart
```

**Rollback:** The pipeline captures the previous image revision label before pulling. If the `/health` endpoint does not respond within 30 seconds, it automatically re-tags and restarts the prior image — no manual intervention required.

**Secrets required in GitHub repository settings:**

| Secret | Description |
|---|---|
| `VULTR_HOST` | IP address of the Vultr VPS |
| `VULTR_USER` | SSH username on the VPS |
| `VULTR_SSH_KEY` | Private SSH key for deploy access |

---

## Project Structure

```
outbound-api/
├── cmd/
│   └── server/
│       └── main.go          # Entry point — wires all components, HTTP server, graceful shutdown
├── internal/
│   ├── alpaca/              # Broker REST client, IEX bar stream, trade-update stream, event mapper
│   ├── bot/                 # Atomic FSM controller (running / paused / stopped)
│   ├── config/              # Env loading, validation, typed Config struct
│   ├── database/            # pgxpool setup, golang-migrate runner, Transactor helper
│   ├── http/                # BotHandlers + RequireAPIKey middleware
│   ├── indicator/           # EMA / RSI / ATR math + thread-safe IndicatorCache
│   ├── model/               # Domain types (Signal, Trade, AccountTransfer) + enums
│   ├── repository/          # DB queries per entity; shared DBTX / transaction context
│   ├── sentiment/           # Alpaca News keyword scoring + CachedProvider (TTL)
│   └── service/             # Business logic: SignalService, TradeService, WatchlistService
├── migrations/
│   ├── embed.go             # embed.FS declaration for iofs driver
│   └── 000000_*.sql …      # Numbered up/down SQL files (auto-applied on startup)
├── .github/
│   └── workflows/
│       └── deploy.yml       # CI: test → build → push to GHCR → deploy to Vultr
├── Dockerfile               # Multi-stage build → scratch image (CGO_ENABLED=0)
├── .env.example             # Template — copy to .env and fill in secrets
└── go.mod
```

---

## License

MIT © [wu-piyaphon](https://github.com/wu-piyaphon)

---

*Built with [Alpaca Markets API](https://docs.alpaca.markets) · Deployed on [Vultr](https://www.vultr.com)*
