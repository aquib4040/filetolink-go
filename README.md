# ⚡ FileToLink-Go Edition

> **High-Performance Telegram MTProto Direct Download & Streaming Engine written in Pure Go.**  
> Written in Go for blazing fast speeds, extreme concurrency, and an ultra-low memory footprint (~20–40 MB RAM) suitable for free and low-RAM cloud containers (Heroku container, Render, Koyeb, Docker, VPS). Zero CGO, stateless AES-256 encrypted URLs, multi-worker parallel chunk fetching, and plug-and-play REST APIs.

[![CI Pipeline](https://github.com/aquib4040/filetolink-go/actions/workflows/ci.yml/badge.svg)](https://github.com/aquib4040/filetolink-go/actions/workflows/ci.yml)
[![Release Builds](https://github.com/aquib4040/filetolink-go/actions/workflows/release.yml/badge.svg)](https://github.com/aquib4040/filetolink-go/actions/workflows/release.yml)
[![Telegram Channel](https://img.shields.io/badge/Telegram-Channel-blue.svg?logo=telegram)](https://t.me/Canon_Bots)
[![Developer](https://img.shields.io/badge/Developer-@ExE__AQUIB-orange.svg?logo=telegram)](https://t.me/ExE_AQUIB)

---

## 👨‍💻 Developer & Community
- 👤 **Developer / Owner:** [@ExE_AQUIB](https://t.me/ExE_AQUIB)
- 📢 **Updates Channel:** [@Canon_Bots](https://t.me/Canon_Bots)
- 📦 **GitHub Repository:** [aquib4040/filetolink-go](https://github.com/aquib4040/filetolink-go)

---

## 🚀 Key Features

- 🏎️ **Pure Go Performance**: Built with zero CGO dependencies, minimal CPU overhead, and ultra-low RAM (~20–40 MB), perfect for low-spec cloud containers.
- 🔗 **Stateless AES-256-GCM Encrypted Links**: Complete zero-database token generation. All message coordinates, hashes, and metadata are authenticated and encrypted inside the link path (`/dl/{token}` and `/watch/{token}`).
- ⚡ **Multi-Worker Parallel Downloader**: Divides files into 1 MiB chunks streamed in parallel across multiple bot sessions with automatic in-flight `FILE_REFERENCE_EXPIRED` refreshing and connection reset recovery.
- 🤖 **Parallel Multi-Bot Session Pool**: Dynamically balances streaming bandwidth across secondary bot tokens (`MULTI_TOKEN1..100`). Authenticates all tokens concurrently in parallel and skips invalid/revoked tokens automatically.
- 🎧 **On-the-Fly Audio & Subtitle Switching**: FFmpeg remuxing pipeline allowing video playback with user-selectable audio and subtitle tracks.
- 📊 **Bandwidth & Real-Time Worker Analytics**:
  - `/status`: Shows live connection breakdown across individual bot tokens.
  - `/stats`: Shows daily, weekly, monthly, yearly, and all-time bandwidth transferred.
  - `/users`: Displays total registered users in MongoDB.
- 🛡️ **Built-in DDoS & Security**: IP-based token-bucket rate limiting, dynamic Force-Subscription (FSub), user ban system, and group chat authorization.

---

## 📖 Bot Commands Reference

| Command | Permission | Description |
| :--- | :---: | :--- |
| `/start` | Public | Start the bot, verify DM permissions, or process verification token |
| `/help` | Public | Comprehensive guide on bot features and group usage |
| `/about` | Public | View engine details, Go architecture highlights, and developer links |
| `/ping` | Public | Test bot latency and cloud server response time |
| `/link` | Public | Reply to any media in an authorized group to generate links |
| `/batch` | Public | Process consecutive files in batch (toggleable via `BATCH` in `.env`) |
| `/status` | **(Admin)** | View real-time active streaming workload per bot worker token |
| `/users` | **(Admin)** | View total registered bot users in MongoDB |
| `/stats` | **(Admin)** | View transferred bandwidth metrics (today, week, month, year, overall) |
| `/speedtest` | **(Admin)** | Run network latency, download, and upload speed benchmark |
| `/settings` | **(Admin)** | Interactive panel (toggle shortener, token auth, TTL, PM mode, batch) |
| `/set_shortener <s\|k>` | **(Admin)** | Update shortener site domain and API key credentials |
| `/set_ttl <hours>` | **(Admin)** | Set token verification duration (e.g. 24 hours) |
| `/fsub` | **(Admin)** | Interactive Force-Sub management panel |
| `/set_fsub <id> <link>`| **(Admin)** | Add a new required Force-Sub channel |
| `/rm_fsub <id>` | **(Admin)** | Remove a Force-Sub channel from monitoring |
| `/ban <id> [reason]` | **(Admin)** | Ban a user ID from accessing bot services |
| `/unban <id>` | **(Admin)** | Unban a previously banned user |
| `/listbanned` | **(Admin)** | List all currently banned users with timestamps |
| `/auth_gc` | **(Admin)** | Authorize a Telegram group chat for link generation |
| `/deauth_gc` | **(Admin)** | Deauthorize a Telegram group chat |
| `/listauth_gc` | **(Admin)** | List all currently authorized group chats |
| `/addpaid <id> <dur>` | **(Admin)** | Grant paid subscription with flexible duration (`30d`, `1m`, `3600s`) |
| `/removepaid <id>` | **(Admin)** | Revoke a user's paid subscription |
| `/listpaid` | **(Admin)** | List all active paid subscribers and expiry dates |
| `/pmmode <on\|off>` | **(Admin)** | Toggle whether non-paid users can generate links in private DM |
| `/restart` | **(Admin)** | Supervised process restart with completion notification |

---

## 🔗 Real Example Links & Verification Workflow

### 1. Shortener Token Verification Flow
When `TOKEN_ENABLED=True` is enabled in `.env` or via `/settings`:
1. **Bot Generates Deep-Link Verification URL**:
   ```
   https://t.me/YourBot?start=verify_64a9f1b2c3d4e5f6a7b8c9d0e1f2a3b4
   ```
2. **Bot Wraps URL via Shortener API (e.g. ShareUS / AdLinkFly)**:
   ```
   https://shareus.io/api?api=YOUR_API_KEY&url=https%3A%2F%2Ft.me%2FYourBot%3Fstart%3Dverify_64a9f1b2c3d4e5f6a7b8c9d0e1f2a3b4
   ```
   *Resulting shortened link presented to user:*
   ```
   https://shareus.io/v/TokenVerify789
   ```
3. **User Unlocks Access**:
   The user opens the link, completes the shortener steps, and is redirected back to Telegram with `/start verify_64a9f1b2...`. The bot activates their session and unlocks link generation for `TOKEN_TTL_HOURS` (e.g. 24 hours).

---

### 2. Media Link Format
- **Direct High-Speed Download:**
  ```
  https://stream.yourdomain.com/dl/dGhpc2lzYW5leGFtcGxlc3RhdGVsZXNzdG9rZW4...
  ```
  *Streams raw binary data with RFC 7233 byte-range support (compatible with IDM, aria2, VLC, and mobile browsers).*
- **Web Video Player:**
  ```
  https://stream.yourdomain.com/watch/dGhpc2lzYW5leGFtcGxlc3RhdGVsZXNzdG9rZW4...
  ```
  *Opens the responsive web player with multi-audio, subtitle switching, and instant scrub/seek.*

---

## 🔀 Permanent URL 302 Redirectors (`redirectors/`)

Never worry about backend container URLs changing. Deploy a lightweight redirector to your custom domain (e.g. `watch.mybrand.com`), set `PERMANENT_REDIRECT_URL=https://watch.mybrand.com`, and all generated links will point to your permanent domain.

The redirector receives `/dl/<token>` or `/watch/<token>` and instantly returns an HTTP `302 Found` to your active backend!

Ready-to-deploy configs are in the [`redirectors/`](./redirectors) folder:
- **[Vercel](./redirectors/vercel/)**: Edge function or `vercel.json` with `TARGET_FQDN` environment variable.
- **[Cloudflare Worker](./redirectors/cloudflare_worker/)**: High-speed edge redirect in `worker.js`.
- **[Cloudflare Pages](./redirectors/cloudflare_pages/)**: Fast static `_redirects` or `functions/[[path]].js`.
- **[Netlify](./redirectors/netlify/)**: Direct rule in `netlify.toml` / `_redirects`.
- **[GitHub Pages](./redirectors/github_pages/)**: Client-side instant redirection via `index.html` and `404.html`.

---

## 🌐 Plug-and-Play REST API

FileToLink-Go Edition includes an HTTP REST API for seamless integration with external apps, websites, and media players:

### 1. Generate Stateless Link
**Endpoint:** `POST /api/generate_link`  
**Headers:** `Content-Type: application/json`

**Request Body:**
```json
{
  "chat_id": -1001234567890,
  "message_id": 4567,
  "file_name": "Sample.Movie.2026.1080p.mkv",
  "file_size": 1572864000
}
```

**Response:**
```json
{
  "success": true,
  "token": "dGhpc2lzYW5leGFtcGxlc3RhdGVsZXNzdG9rZW4...",
  "stream_url": "https://stream.yourdomain.com/watch/dGhpc2lzYW5leGFtcGxlc3RhdGVsZXNzdG9rZW4...",
  "download_url": "https://stream.yourdomain.com/dl/dGhpc2lzYW5leGFtcGxlc3RhdGVsZXNzdG9rZW4..."
}
```

---

### 2. Audio & Subtitle Track Discovery
**Endpoint:** `GET /api/tracks/{token}`

Dynamically probes the media file via `ffprobe` and returns all available audio languages and subtitle tracks:
```json
{
  "audio_tracks": [
    {"index": 1, "language": "jpn", "title": "Japanese (Original)", "codec": "aac"},
    {"index": 2, "language": "eng", "title": "English Dub", "codec": "aac"}
  ],
  "subtitle_tracks": [
    {"index": 0, "language": "eng", "title": "English Full Subs"}
  ]
}
```

---

### 3. System Analytics & Health Probe
- `GET /stats`: Real-time transferred bandwidth counters and active session pool status.
- `GET /health`: Liveness probe for load balancers and container orchestrators (returns `200 OK`).

---

## 🐳 Docker Deployment

1. Clone the repository and configure your environment:
   ```bash
   git clone https://github.com/aquib4040/filetolink-go.git
   cd filetolink-go
   cp .env.example .env
   # Edit .env with your Telegram credentials and MongoDB URI
   ```

2. Generate a 32-byte AES key:
   - Open `web/key_generator.html` in any browser or generate via OpenSSL:
     ```bash
     openssl rand -base64 32
     ```
   - Paste into `ENCRYPTION_KEY` in `.env`.

3. Launch the container:
   ```bash
   docker compose up -d --build
   docker compose logs -f
   ```

---

## ☁️ Heroku Deployment

FileToLink-Go Edition is optimized for Heroku Container Stack with dynamic port binding (`$PORT`) and automatic domain detection.

### Automatic Deployment via GitHub Actions
1. Fork or clone this repository to GitHub.
2. In GitHub repository settings, navigate to **Settings > Secrets and variables > Actions**.
3. Add secrets:
   - `HEROKU_API_KEY`: Your Heroku API key
   - `HEROKU_APP_NAME`: Your Heroku app name
   - Plus your required config variables (`API_ID`, `API_HASH`, `BOT_TOKEN`, `BIN_CHANNEL`, `ENCRYPTION_KEY`, `DATABASE_URL`, etc.)
4. The deployment workflow in `.github/workflows/heroku.yml` will automatically:
   - Install the official Heroku CLI.
   - Build and push the Docker container to the Heroku registry.
   - Query the Heroku API to detect your app's web URL.
   - Automatically configure `FQDN` to match your Heroku web URL without manual setup.
   - Release the container with zero downtime.

---

## 📌 Optional Configuration Notes

> [!NOTE]
> **Reel Channel (`REEL_CHANNEL_ID`)**:
> The `REEL_CHANNEL_ID` setting is **completely optional** and intended primarily for personal media cataloging. If set, media posted in this channel is automatically indexed in MongoDB. If you do not need this feature, simply leave `REEL_CHANNEL_ID=0`.

---

## 📜 License

FileToLink-Go Edition is distributed under the [MIT License](LICENSE).
