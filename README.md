<p align="center">
  <img src="./web/logo.png" alt="FileToLink-Go Logo" width="130" style="border-radius: 20px;">
  <h1 align="center">FileToLink-Go</h1>
</p>

<p align="center">
  <b>Enterprise-Grade Telegram MTProto Direct Download & Streaming Engine written in Pure Go</b>
</p>

<p align="center">
  <a href="https://github.com/aquib4040/filetolink-go/actions/workflows/ci.yml"><img src="https://github.com/aquib4040/filetolink-go/actions/workflows/ci.yml/badge.svg" alt="CI Pipeline"></a>
  <img src="https://img.shields.io/badge/Go-1.26-00ADD8?style=flat&logo=go&logoColor=white" alt="Go Version">
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-blue?style=flat" alt="License"></a>
  <a href="https://t.me/Canon_Bots"><img src="https://img.shields.io/badge/Channel-@Canon__Bots-blue?style=flat&logo=telegram" alt="Telegram Channel"></a>
  <a href="https://t.me/ExE_AQUIB"><img src="https://img.shields.io/badge/Developer-@ExE__AQUIB-orange?style=flat&logo=telegram" alt="Developer"></a>
</p>

<p align="center">
  <a href="https://www.heroku.com/deploy?template=https://github.com/aquib4040/filetolink-go"><img src="https://www.herokucdn.com/deploy/button.svg" alt="Deploy to Heroku"></a>
  <a href="https://render.com/deploy"><img src="https://img.shields.io/badge/Deploy%20to-Render-46E3B7?style=for-the-badge&logo=render&logoColor=white" alt="Deploy to Render"></a>
  <a href="https://app.koyeb.com/deploy"><img src="https://img.shields.io/badge/Deploy%20to-Koyeb-121212?style=for-the-badge&logo=koyeb" alt="Deploy to Koyeb"></a>
</p>

---

## 📑 Table of Contents

- [About](#about)
- [How It Works](#how-it-works)
- [Key Features](#key-features)
- [Architecture & Performance](#architecture--performance)
- [Bot Commands](#bot-commands)
- [REST API Endpoints](#rest-api-endpoints)
- [Configuration (.env)](#configuration-env)
- [Deployment](#deployment)
  - [Heroku Deployment](#heroku-deployment)
  - [Docker & VPS Deployment](#docker--vps-deployment)
- [Edge Redirectors](#edge-redirectors)
- [Developer & Credits](#developer--credits)

---

## 💡 About

**FileToLink-Go** is a high-performance, concurrent Telegram MTProto streaming server and bot engine built from scratch in pure Go. It converts any media, document, video, or archive uploaded to Telegram into high-speed, direct HTTP/HTTPS download and web-player streaming links.

Designed for efficiency, FileToLink-Go consumes an ultra-low memory footprint (~20–40 MB RAM), making it perfect for free and constrained cloud containers (Heroku, Render, Koyeb, Fly.io, or VPS) while achieving maximum multi-gigabit throughput.

---

## 🔄 How It Works

```
 User               Bot                Storage Channel        MongoDB           Browser / IDM
  │                  │                        │                  │                    │
  │── Send Media ──▶ │                        │                  │                    │
  │                  │── Forward (Copy) ────▶ │                  │                    │
  │                  │── Reply with Info ───▶ │                  │                    │
  │                  │                                           │                    │
  │◀── Return Link ──│ (Stateless AES-256 Token)                 │                    │
  │                                                              │                    │
  │──────────────────────── GET /dl/{token} or /watch/{token} ──────────────────────▶ │
                     │                        │                  │                    │
                     │◀── Multi-Worker Pool ──│                  │                    │
                     │    (16+ Bot Tokens)    │                  │                    │
                     │                                           │                    │
                     │════════════ Stream Parallel 1 MiB Chunks ════════════════════▶ │
```

1. **Zero-Database Stateless Link Generation**: All file coordinates, hashes, and sizes are AES-256-GCM encrypted into the URL token. Links never expire and require no database lookups to stream.
2. **True Message Copy**: Files stored in the storage channel use `DropAuthor: true`, keeping the storage channel clean without "Forwarded from" tags.
3. **Multi-Token Parallel Fetching**: Incoming HTTP requests trigger multi-worker downloads across up to 24+ pooled Telegram bot tokens in parallel, bypassing single-account speed throttles.

---

## ✨ Key Features

### 🏎️ High-Performance Core
- **Pure Go (Zero CGO)**: Compiles into a single static binary for Linux, macOS, and Windows.
- **Ultra-Low Resource Footprint**: Operates stably with ~20–40 MB RAM under load.
- **Dynamic Channel AccessHash Resolution**: Automatically queries Telegram for channel access hashes on startup and in-flight, preventing MTProto `CHANNEL_INVALID` (400) errors.

### 🌐 Streaming & Media Player
- **RFC 7233 Byte-Range Support**: Seamless seeking, pause, resume, and multi-connection acceleration with IDM, aria2, and VLC.
- **Interactive Dark-Mode Web Player**: Built-in HTML5 media player supporting custom aspect ratios and subtitles.
- **On-the-Fly Audio & Subtitle Remuxing**: Powered by an optional FFmpeg pipeline allowing users to dynamically switch audio tracks and subtitles directly in the browser.

### 🛡️ Smart Bot Features & Resilience
- **Interactive About & Help Panels**: Fully navigable inline keyboard system with Back (`⬅️ Back`) and Close (`❌ Close`) actions.
- **Random Reel Media Replies**: Supports optional promotional/reel channels, automatically replying to commands with random video/photo reels.
- **Auto Text & Caption Splitting**: 
  - Captions exceeding Telegram's 1024-character limit are automatically detached and sent as clean follow-up messages.
  - Long texts exceeding 4096 characters are intelligently split at natural line breaks without disrupting markdown syntax.
- **Dyno Keepalive Loop**: Automatically pings the public endpoint every 15 minutes to keep free/hobby cloud dynos awake.

---

## ⚡ Architecture & Performance

| Metric | Traditional Python Engines | FileToLink-Go |
| :--- | :--- | :--- |
| **Startup RAM** | ~180 – 350 MB | **~22 MB** |
| **Concurrency Model** | Python Asyncio GIL | **Goroutines + Channel Workpools** |
| **Token Distribution** | Single Active Worker | **Round-Robin Multi-Token Pool (24+ Bots)** |
| **Chunk Size** | 512 KB | **1024 KB (Max Telegram Throughput)** |
| **FloodWait Recovery** | Sleeps entire stream | **Auto-rotates to next bot session instantly** |
| **Binary Output** | Multiple `.py` files + venv | **Single 25 MB Static Binary** |

---

## 📖 Bot Commands

| Command | Permission | Description |
| :--- | :---: | :--- |
| `/start` | Public | Welcome panel, user registration, and deep-link token verification |
| `/help` | Public | Interactive feature walkthrough with inline navigation |
| `/about` | Public | Engine specifications, developer info, and version |
| `/ping` | Public | Check bot latency and server response time |
| `/link` | Public | Reply to any media file to instantly generate streaming links |
| `/batch` | Public | Process consecutive channel files in batch |
| `/dc` | Public | View current Telegram Data Center and network latency |
| `/status` | **Admin** | Real-time system status, uptime, and workload distribution across all bot instances |
| `/stats` | **Admin** | Live transferred bandwidth analytics (today, weekly, monthly, yearly, overall) |
| `/users` | **Admin** | View total registered users in MongoDB Atlas |
| `/speedtest` | **Admin** | Run an automated network speed test (ping, upload, download) |
| `/settings` | **Admin** | Interactive dashboard to toggle shorteners, token auth, TTL, and batch modes |
| `/set_shortener` | **Admin** | Update URL shortener API credentials |
| `/set_ttl` | **Admin** | Adjust verification token expiry duration (in hours) |
| `/fsub` | **Admin** | Interactive Force-Subscription channel manager |
| `/set_fsub` | **Admin** | Add a new required Force-Sub channel |
| `/rm_fsub` | **Admin** | Remove a Force-Sub channel from monitoring |
| `/ban` / `/unban` | **Admin** | Ban or unban a user ID from accessing bot services |
| `/listbanned` | **Admin** | Display all currently blacklisted users |
| `/auth_gc` / `/deauth_gc`| **Admin** | Manage authorized Telegram groups |
| `/addpaid` / `/removepaid`| **Admin** | Grant or revoke premium subscriptions |
| `/restart` | **Admin** | Perform a zero-downtime supervised application restart |

---

## 🌐 REST API Endpoints

### 1. Direct Download & Streaming
- `GET /dl/{token}` — High-speed binary stream with full byte-range support.
- `GET /watch/{token}` — Responsive web video player interface.
- `GET /reel_random` — Returns a random message ID from the indexed reel media collection:
  ```json
  {
    "success": true,
    "message_id": 1420
  }
  ```

### 2. Programmatic Link Generation
- `POST /api/generate_link` — Generate encrypted stateless links via REST:
  ```json
  {
    "chat_id": -1001234567890,
    "message_id": 5678,
    "file_name": "Sample.mkv",
    "file_size": 104857600
  }
  ```

### 3. Audio & Subtitle Tracks
- `GET /api/tracks/{token}` — Returns available audio languages and subtitle tracks for the media.

### 4. Health & System Metrics
- `GET /stats` — Live bandwidth transfer counters and active streaming sessions.
- `GET /health` — Liveness health-check endpoint (returns `200 OK`).

---

## ⚙️ Configuration (.env)

```env
# --- Core Telegram Credentials ---
API_ID=12345678
API_HASH=your_api_hash_here
BOT_TOKEN=1234567890:ABC-DEF_your_primary_bot_token
BIN_CHANNEL=-1001234567890
OWNER_ID=123456789

# --- Multi-Token Pool (Up to 100+ Bots for Max Speed) ---
MULTI_TOKEN1=token_1
MULTI_TOKEN2=token_2
# ... add as many as needed

# --- Database & Security ---
DATABASE_URL=mongodb+srv://user:pass@cluster.mongodb.net/?retryWrites=true&w=majority
DATABASE_NAME=filetolink_db
ENCRYPTION_KEY=32_byte_base64_encoded_aes_key

# --- Web & Networking ---
FQDN=https://your-domain.herokuapp.com
PORT=8080
BIND_ADDRESS=0.0.0.0
DOWNLOAD_THREADS=16

# --- Optional Configurations ---
REEL_CHANNEL_ID=0
PERMANENT_REDIRECT_URL=
SHORTENER_API=
SHORTENER_URL=
```

---

## 🚀 Deployment

### Heroku Deployment (Recommended)
1. Fork or clone this repository.
2. In your GitHub repository, add your Heroku credentials under **Settings > Secrets and variables > Actions**:
   - `HEROKU_API_KEY`: Your Heroku API key
   - `HEROKU_APP_NAME`: Your Heroku app name
   - Plus all required `.env` secrets (`BOT_TOKEN`, `API_ID`, `API_HASH`, `BIN_CHANNEL`, `ENCRYPTION_KEY`, etc.)
3. The included GitHub Actions workflow (`.github/workflows/heroku.yml`) will automatically build, test, and release the container to Heroku with zero downtime.

### Docker & VPS Deployment
```bash
# 1. Clone repository
git clone https://github.com/aquib4040/filetolink-go.git
cd filetolink-go

# 2. Configure environment
cp .env.example .env
# Fill in your credentials in .env

# 3. Build and launch
docker compose up -d --build
docker compose logs -f
```

---

## 🔀 Edge Redirectors (`redirectors/`)

Deploy permanent custom domains (e.g. `stream.mybrand.com`) using serverless edge redirectors:
- **[Cloudflare Worker](./redirectors/cloudflare_worker/)** — Lightning-fast edge redirect script.
- **[Cloudflare Pages](./redirectors/cloudflare_pages/)** — Free static/edge deployment.
- **[Vercel Edge](./redirectors/vercel/)** — Zero-config edge redirects.
- **[Netlify](./redirectors/netlify/)** — Lightweight `_redirects` configuration.

---

## 👨‍💻 Developer & Community

- **Developer:** [@ExE_AQUIB](https://t.me/ExE_AQUIB)
- **Updates Channel:** [@Canon_Bots](https://t.me/Canon_Bots)
- **GitHub:** [aquib4040/filetolink-go](https://github.com/aquib4040/filetolink-go)

---

## 📜 License

Distributed under the [MIT License](LICENSE).
