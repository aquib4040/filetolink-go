package config

import (
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	APIID                int32
	APIHash              string
	BotToken             string
	OwnerID              int64
	BinChannel           int64
	ReelChannelID        int64
	FQDN                 string
	Port                 int
	HasSSL               bool
	BindAddress          string
	PermanentRedirectURL string
	EncryptionKey        []byte
	BotSigningSecret     string
	DatabaseURL          string
	DownloadThreads      int
	GotdDataPath         string
	MaxBatchFiles        int
	Channel              bool
	PMModeDefault        bool
	Batch                bool
	MultiTokens          []string
	ShortenerSite        string
	ShortenerAPIKey      string
	ShortenMediaLinks    bool
	RateLimitRPS         int
	RateLimitBurst       int
	PingInterval         time.Duration
}

func LoadConfig() (*Config, error) {
	// 1. Capture dynamic PORT injected by cloud platforms like Heroku/Render before reading .env
	dynamicPort := os.Getenv("PORT")
	loadDotenv(".env")
	if dynamicPort != "" {
		_ = os.Setenv("PORT", dynamicPort)
	}

	apiID := int32(0)
	if idStr := os.Getenv("API_ID"); idStr != "" {
		if id, err := strconv.Atoi(idStr); err == nil {
			apiID = int32(id)
		}
	}

	apiHash := os.Getenv("API_HASH")
	botToken := os.Getenv("BOT_TOKEN")
	if apiID == 0 || apiHash == "" || botToken == "" {
		return nil, fmt.Errorf("API_ID, API_HASH, and BOT_TOKEN are required in environment")
	}

	ownerID := int64(0)
	if oStr := os.Getenv("OWNER_ID"); oStr != "" {
		if id, err := strconv.ParseInt(oStr, 10, 64); err == nil {
			ownerID = id
		}
	}

	binChannel := int64(0)
	if bStr := os.Getenv("BIN_CHANNEL"); bStr != "" {
		if id, err := strconv.ParseInt(bStr, 10, 64); err == nil {
			binChannel = id
		}
	}
	if binChannel == 0 {
		return nil, fmt.Errorf("BIN_CHANNEL is required")
	}

	reelChannelID := int64(0)
	if rStr := os.Getenv("REEL_CHANNEL_ID"); rStr != "" {
		if id, err := strconv.ParseInt(rStr, 10, 64); err == nil {
			reelChannelID = id
		}
	}

	// Dynamic port handling for Heroku/Render/Docker:
	port := 8080
	if pStr := os.Getenv("PORT"); pStr != "" {
		if p, err := strconv.Atoi(pStr); err == nil && p > 0 {
			port = p
		}
	}

	fqdn := os.Getenv("FQDN")
	if fqdn == "" || strings.HasPrefix(fqdn, "localhost") {
		if hApp := os.Getenv("HEROKU_APP_NAME"); hApp != "" {
			fqdn = fmt.Sprintf("%s.herokuapp.com", hApp)
		} else if fqdn == "" {
			fqdn = fmt.Sprintf("localhost:%d", port)
		}
	}
	fqdn = strings.TrimPrefix(fqdn, "http://")
	fqdn = strings.TrimPrefix(fqdn, "https://")
	fqdn = strings.TrimSuffix(fqdn, "/")

	hasSSL := true
	if sslStr := os.Getenv("HAS_SSL"); sslStr != "" {
		hasSSL = strings.ToLower(sslStr) == "true" || sslStr == "1"
	}

	bindAddress := os.Getenv("BIND_ADDRESS")
	if bindAddress == "" {
		bindAddress = "0.0.0.0"
	}

	permRedirect := os.Getenv("PERMANENT_REDIRECT_URL")
	permRedirect = strings.TrimSuffix(permRedirect, "/")

	// Cryptography Key - Derive exact 32 bytes for AES-256-GCM
	rawKey := os.Getenv("ENCRYPTION_KEY")
	var keyBytes []byte
	if rawKey != "" {
		if decoded, err := base64.StdEncoding.DecodeString(rawKey); err == nil && len(decoded) == 32 {
			keyBytes = decoded
		} else if decoded, err := base64.URLEncoding.DecodeString(rawKey); err == nil && len(decoded) == 32 {
			keyBytes = decoded
		} else {
			h := sha256.Sum256([]byte(rawKey))
			keyBytes = h[:]
		}
	} else {
		h := sha256.Sum256([]byte("default-filetolink-encryption-secret-key-32b"))
		keyBytes = h[:]
	}

	signingSecret := os.Getenv("BOT_SIGNING_SECRET")
	if signingSecret == "" {
		signingSecret = "filetolink-signing-secret"
	}

	dbURL := os.Getenv("DATABASE_URL")

	threads := 16
	if thStr := os.Getenv("DOWNLOAD_THREADS"); thStr != "" {
		if th, err := strconv.Atoi(thStr); err == nil && th > 0 {
			threads = th
		}
	}

	gotdPath := os.Getenv("GOTD_DATA_PATH")
	if gotdPath == "" {
		gotdPath = "./data/gotd"
	}

	maxBatch := 20
	if mbStr := os.Getenv("MAX_BATCH_FILES"); mbStr != "" {
		if mb, err := strconv.Atoi(mbStr); err == nil && mb > 0 {
			maxBatch = mb
		}
	}

	channelMode := strings.ToLower(os.Getenv("CHANNEL")) == "true"
	pmMode := strings.ToLower(os.Getenv("PM_MODE_DEFAULT")) == "true"
	batchMode := true
	if bStr := os.Getenv("BATCH"); bStr != "" {
		batchMode = strings.ToLower(bStr) == "true" || bStr == "1"
	}

	// Collect multi-tokens
	var multiTokens []string
	for i := 1; i <= 100; i++ {
		key := fmt.Sprintf("MULTI_TOKEN%d", i)
		if val := strings.Trim(strings.TrimSpace(os.Getenv(key)), `"'`); val != "" {
			multiTokens = append(multiTokens, val)
		}
	}

	shortenerSite := os.Getenv("URL_SHORTENER_SITE")
	shortenerAPIKey := os.Getenv("URL_SHORTENER_API_KEY")
	shortenLinks := strings.ToLower(os.Getenv("SHORTEN_MEDIA_LINKS")) == "true"

	rateLimitRPS := 10
	if rStr := os.Getenv("RATE_LIMIT_RPS"); rStr != "" {
		if r, err := strconv.Atoi(rStr); err == nil && r > 0 {
			rateLimitRPS = r
		}
	}

	rateLimitBurst := 20
	if bStr := os.Getenv("RATE_LIMIT_BURST"); bStr != "" {
		if b, err := strconv.Atoi(bStr); err == nil && b > 0 {
			rateLimitBurst = b
		}
	}

	pingInterval := 10 * time.Minute
	if piStr := os.Getenv("PING_INTERVAL"); piStr != "" {
		if sec, err := strconv.Atoi(piStr); err == nil && sec > 0 {
			pingInterval = time.Duration(sec) * time.Second
		}
	}

	return &Config{
		APIID:                apiID,
		APIHash:              apiHash,
		BotToken:             botToken,
		OwnerID:              ownerID,
		BinChannel:           binChannel,
		ReelChannelID:        reelChannelID,
		FQDN:                 fqdn,
		Port:                 port,
		HasSSL:               hasSSL,
		BindAddress:          bindAddress,
		PermanentRedirectURL: permRedirect,
		EncryptionKey:        keyBytes,
		BotSigningSecret:     signingSecret,
		DatabaseURL:          dbURL,
		DownloadThreads:      threads,
		GotdDataPath:         gotdPath,
		MaxBatchFiles:        maxBatch,
		Channel:              channelMode,
		PMModeDefault:        pmMode,
		Batch:                batchMode,
		MultiTokens:          multiTokens,
		ShortenerSite:        shortenerSite,
		ShortenerAPIKey:      shortenerAPIKey,
		ShortenMediaLinks:    shortenLinks,
		RateLimitRPS:         rateLimitRPS,
		RateLimitBurst:       rateLimitBurst,
		PingInterval:         pingInterval,
	}, nil
}

// BuildBaseURL returns the direct streaming origin URL (e.g. "https://my-app.herokuapp.com" or "http://localhost:8080")
func (c *Config) BuildBaseURL() string {
	proto := "http"
	if c.HasSSL {
		proto = "https"
	}
	return fmt.Sprintf("%s://%s", proto, c.FQDN)
}

// BuildEffectiveBaseURL returns the permanent redirect URL if configured, otherwise BuildBaseURL
func (c *Config) BuildEffectiveBaseURL() string {
	if c.PermanentRedirectURL != "" {
		return c.PermanentRedirectURL
	}
	return c.BuildBaseURL()
}

func loadDotenv(filename string) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			k := strings.TrimSpace(parts[0])
			v := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
			// Only set if not already set in OS environment
			if os.Getenv(k) == "" {
				_ = os.Setenv(k, v)
			}
		}
	}
}
