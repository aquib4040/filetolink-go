# ⚡ FileToLink Go

High-performance, production-ready Telegram file-to-link streaming and download service written in Go.

Powered by `gotd/td` MTProto, parallel chunk streaming with zero-CGO, and a stateless encrypted link architecture.

---

## 🌟 Key Features

- **Pure Go & Zero CGO**: Built on `gotd/td` MTProto client, consuming minimal memory (~10 MB RAM per worker session).
- **Stateless Encrypted Link Generation**: Streaming and download links do **not** expose database IDs, file hashes, or file names. Everything is encrypted statelessly via AES-256-GCM.
- **Hidden Hashes & Names in URLs**: Clean, permanent links (`/watch/{token}` and `/dl/{token}`). File names and hashes are strictly sent via HTTP `Content-Disposition` on download.
- **High-Throughput Parallel Fetcher**: Splits files into aligned 1 MiB Telegram parts across configurable download threads (`DOWNLOAD_THREADS=16`) with automatic in-flight `FILE_REFERENCE_EXPIRED` refreshing.
- **Multi-Bot Session Pool**: Dynamically balances streaming workload across dozens of worker bots (`MULTI_TOKEN1..N`) with instant timeout detection and in-memory skipping for invalid tokens.
- **FFmpeg On-the-Fly Remuxing**: Real-time audio track and subtitle switching with fast `-ss` keyframe seeking.
- **Bandwidth & User Analytics**: MongoDB integration for tracking streamed bytes across daily, weekly, monthly, yearly, and all-time windows, as well as tracking users and group authorizations.
- **Start in DM & Dynamic FSub**: Forces users in groups to start the bot in private first before receiving links, and verifies dynamic force-subscription channels.
- **Colorful Telegram Buttons**: High-contrast UI with success (green), danger (red), and primary (blue) button styles and small-caps typography.
- **Full REST API Suite**: Includes `/api/generate_link`, `/api/file_stream_url`, `/api/stream/{chat_id}/{path}`, `/api/tracks/{token}`, `/stats`, and `/health`.

---

## 🚀 Environment Configuration

Copy `.env.example` to `.env` and configure your credentials:

```bash
# Telegram Core API
API_ID=12345678
API_HASH=your_telegram_api_hash
BOT_TOKEN=1234567890:ABCdefGHIjklMNOpqrSTUvwxYZ
OWNER_ID=123456789

# Storage & Channels
BIN_CHANNEL=-1001234567890

# Cryptography (32-byte secret key)
ENCRYPTION_KEY=32_character_or_base64_secret_key

# Web Server & FQDN
PORT=8080
FQDN=your-domain.com
HAS_SSL=true

# MongoDB Database URL
DATABASE_URL=mongodb+srv://...

# Multi-Bot Worker Tokens
MULTI_TOKEN1=...
MULTI_TOKEN2=...
```

---

## 🛠️ Running Locally

### With Go
```bash
go run ./cmd/server
```

### With Docker
```bash
docker-compose up --build -d
```

---

## ☁️ Deployment

### Heroku Container Stack
This project includes a native `heroku.yml` configured for container deployments.

1. Set the Heroku app stack to `container`:
   ```bash
   heroku stack:set container -a your-app-name
   ```
2. Push and deploy:
   ```bash
   git push heroku main
   ```

---

## 📜 Commands Reference

| Command | Description |
| :--- | :--- |
| `/start` | Start the bot and get your user ID recorded |
| `/help` | View help and usage instructions |
| `/ping` | Check bot latency and server status |
| `/link` | Generate permanent links for replied media in authorized groups |
| `/auth_gc [chat_id]` | *(Owner)* Authorize a group chat |
| `/deauth_gc [chat_id]` | *(Owner)* Deauthorize a group chat |
| `/listauth_gc` | *(Owner)* List all authorized groups |
| `/addpaid <user_id> <duration>` | *(Owner)* Add paid subscription (e.g. `30d`, `1m`, `3600s`) |
| `/removepaid <user_id>` | *(Owner)* Revoke user paid status |
| `/listpaid` | *(Owner)* View all active paid users |
| `/pmmode [on\|off]` | *(Owner)* Toggle whether all users can use PM without paid status |
| `/stats` | *(Owner)* View detailed bandwidth and user metrics |

---

## 📄 License
MIT License
