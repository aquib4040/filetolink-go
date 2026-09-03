# ⚡ FileToLink Go (Pro Edition)

> **High-Performance Telegram MTProto Streaming & Direct Download Engine written in Pure Go.**
> Zero CGO, low memory footprint (~10 MB RAM per session), 16-thread chunk division, stateless AES-256 encrypted URLs, on-the-fly multi-audio & subtitle switching, and plug-and-play REST APIs.

[![CI Pipeline](https://github.com/aquib4040/filetolink-go/actions/workflows/ci.yml/badge.svg)](https://github.com/aquib4040/filetolink-go/actions/workflows/ci.yml)
[![Release Builds](https://github.com/aquib4040/filetolink-go/actions/workflows/release.yml/badge.svg)](https://github.com/aquib4040/filetolink-go/actions/workflows/release.yml)
[![Telegram Channel](https://img.shields.io/badge/Telegram-Channel-blue.svg?logo=telegram)](https://t.me/Anime_Canon)

---

## 🚀 Key Features

- 🏎️ **Pure Go MTProto Performance**: Built upon `gotd/td` v0.161.0. Zero CGO dependencies, ultra-fast binary execution, and minimal memory usage.
- 🔗 **Stateless AES-256-GCM Encrypted Links**: Complete zero-database token generation. All message coordinates, hashes, and metadata are authenticated and encrypted inside the link path (`/watch/{token}` and `/dl/{token}`). File names and hashes are never exposed in URLs.
- ⚡ **16-Thread Parallel Downloader**: Divides files into 1 MiB chunks streamed in parallel across multiple bot sessions with automatic in-flight `FILE_REFERENCE_EXPIRED` refreshing and connection reset recovery.
- 🎧 **On-the-Fly Audio & Subtitle Switching**: FFmpeg remuxing pipeline allowing video streaming with user-selectable audio and subtitle tracks without restarting the playback stream.
- 🤖 **Multi-Bot Session Pool**: Dynamically balances streaming bandwidth across secondary bot tokens (`MULTI_TOKEN1..100`). Automatically detects and skips invalid or revoked tokens in memory without startup delays.
- 📊 **Bandwidth & User Analytics**: Tracks daily, weekly, monthly, yearly, and all-time bandwidth transfer in MongoDB Atlas.
- 🎨 **Modern Telegram UI**: Colorful inline buttons (`StyleGreen`, `StyleBlue`, `StyleRed`) ported from FileStore.
- 🛠️ **Plug-and-Play REST APIs**: Direct HTTP endpoints to generate and stream files via code or external frontends.
- 🛡️ **Built-in Security**: IP-based rate limiting, dynamic Force-Subscription (FSub), user ban system, and group authorization.

---

## 📖 Bot Commands Reference

| Command | Permission | Description |
| :--- | :---: | :--- |
| `/start` | Public | Start the bot, register user, and verify DM start permissions |
| `/help` | Public | Comprehensive guide on bot features and group usage |
| `/ping` | Public | Test bot latency and cloud server response time |
| `/link` | Public | Reply to any media in an authorized group to generate links |
| `/batch` | Public | Process consecutive files in batch (configurable via `BATCH` in `.env`) |
| `/about` | Public | View bot version, engine details, and developer credits |
| `/stats` | **(Admin)** | View active bot pool sessions and transferred bandwidth metrics |
| `/speedtest` | **(Admin)** | Run network latency and download speed benchmark |
| `/settings` | **(Admin)** | Interactive control panel (toggle shortener, token auth, TTL, PM mode, batch) |
| `/set_shortener <s|k>` | **(Admin)** | Update shortener site and API key credentials |
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

## 🌐 Plug-and-Play REST API

FileToLink Go includes a comprehensive HTTP REST API for seamless integration with external apps, websites, and media players:

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

### 2. Resolve File Stream URL (Plug-and-Play)
**Endpoint:** `GET /api/file_stream_url?chat_id={chat_id}&message_id={message_id}`

Returns instant direct download and stream URLs for any Telegram message coordinates:
```json
{
  "success": true,
  "stream_url": "https://stream.yourdomain.com/watch/{token}",
  "download_url": "https://stream.yourdomain.com/dl/{token}"
}
```

---

### 3. Stream or Download via Encrypted Token
- **Web Video Player:** `GET /watch/{token}`
  - Includes custom HTML5 video player, audio track selector, subtitle switcher, and instant seeking.
- **Direct High-Speed Download:** `GET /dl/{token}`
  - Supports RFC 7233 HTTP `Range` requests, multi-thread download managers (IDM, aria2, curl), and native browser downloads.

---

### 4. Audio & Subtitle Track Discovery
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

### 5. System Analytics & Health Probe
- `GET /stats`: Real-time transferred bandwidth counters and active session pool status.
- `GET /health`: Liveness probe for load balancers and container orchestrators (returns `200 OK`).

---

## 🐳 Docker Deployment

### Using Docker Compose (Recommended)

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
   - Paste the key into `ENCRYPTION_KEY` inside `.env`.

3. Launch the container:
   ```bash
   docker compose up -d --build
   ```

4. View service logs:
   ```bash
   docker compose logs -f
   ```

---

## ☁️ Heroku Deployment

FileToLink Go is optimized for Heroku Container Stack with dynamic port binding (`$PORT`) and automatic domain detection.

### Automatic Deployment via GitHub Actions
1. Fork or clone this repository to GitHub.
2. In your GitHub repository, navigate to **Settings > Secrets and variables > Actions**.
3. Add the following secrets:
   - `HEROKU_API_KEY`: Your Heroku API key
   - `HEROKU_APP_NAME`: Your Heroku app name
   - Plus your environment variables (`API_ID`, `API_HASH`, `BOT_TOKEN`, `BIN_CHANNEL`, `ENCRYPTION_KEY`, `DATABASE_URL`, etc.)
4. The deployment workflow in `.github/workflows/heroku.yml` will automatically:
   - Build and push the Docker container to the Heroku registry.
   - Query the Heroku API to detect your app's web URL.
   - Automatically configure `FQDN` to match your Heroku web URL without manual setup.
   - Release the container with zero downtime.

---

## 🔒 Security & Privacy Architecture

- **No Stored Links**: Files are not mapped to database IDs. URLs are cryptographically self-contained tokens.
- **Hidden Metadata**: File names and internal hashes are never exposed in public link URLs; file names are passed safely via standard HTTP `Content-Disposition`.
- **In-Memory Rate Limiting**: Built-in token-bucket algorithm per client IP mitigates DDoS and scraping attacks.
- **Non-Streamable File Filtering**: Non-media files (e.g. `.zip`, `.rar`, `.tar.gz`, `.exe`, `.apk`, `.iso`) only generate download links and omit video streaming buttons.

---

## 📌 Optional Configuration Notes

> [!NOTE]
> **Reel Channel (`REEL_CHANNEL_ID`)**:
> The `REEL_CHANNEL_ID` setting is **completely optional** and intended primarily for personal media cataloging. If set, media posted in this channel is automatically indexed in MongoDB. If you do not need this feature, simply leave `REEL_CHANNEL_ID=0`.

---

## 🤝 Community & Support

Join our Telegram channel for updates, help, and community discussions:
- 📢 **Official Channel:** [@Anime_Canon](https://t.me/Anime_Canon)

---

## 📜 License

FileToLink Go is distributed under the [MIT License](LICENSE).

