package stream

import (
	"context"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"filetolink-go/internal/config"
	"filetolink-go/internal/crypto"
	"filetolink-go/internal/db"
	"filetolink-go/internal/pool"
)

type MediaForwarder interface {
	ForwardAndGenerateLink(ctx context.Context, fromChannelID int64, messageID int64) (downloadURL string, streamURL string, fileName string, fileSize int64, err error)
}

type HTTPServer struct {
	cfg          *config.Config
	pool         *pool.SessionPool
	fetcher      *ParallelFetcher
	database     *db.BotDatabase
	templates    *template.Template
	forwarder    MediaForwarder
	
	activeDownloads  atomic.Int64
	totalDownloads   atomic.Int64
	totalBytesServed atomic.Int64

	rateMu     sync.Mutex
	requestLog map[string][]int64
}

func (s *HTTPServer) SetForwarder(f MediaForwarder) {
	s.forwarder = f
}

func NewHTTPServer(cfg *config.Config, p *pool.SessionPool, d *db.BotDatabase) *HTTPServer {
	// Parse HTML templates from web/template with standard fallback functions
	tmpl, err := template.New("").Funcs(template.FuncMap{
		"file_name": func() string { return "" },
		"src":       func() string { return "" },
		"slice": func(s string, start, end int) string {
			if start >= len(s) {
				return ""
			}
			if end > len(s) {
				end = len(s)
			}
			return s[start:end]
		},
	}).ParseGlob("web/template/*.html")
	if err != nil {
		log.Printf("[HTTPServer] Warning: failed to parse templates from web/template: %v", err)
	}

	return &HTTPServer{
		cfg:        cfg,
		pool:       p,
		fetcher:    NewParallelFetcher(p),
		database:   d,
		templates:  tmpl,
		requestLog: make(map[string][]int64),
	}
}

func (s *HTTPServer) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// Web Player (/watch/) & Downloads (/dl/)
	mux.HandleFunc("/watch/", s.handleWatch)
	mux.HandleFunc("/dl/", s.handleDownload)
	mux.HandleFunc("/watch", s.handlePermanentRedirect)
	mux.HandleFunc("/dl", s.handlePermanentRedirect)

	// API endpoints (Unified single link generation endpoint)
	mux.HandleFunc("/reel_random", s.handleReelRandom)
	mux.HandleFunc("/api/generate_link", s.handleAPIGenerateLink)
	mux.HandleFunc("/api/file_stream_url", s.handleAPIGenerateLink)
	mux.HandleFunc("/api/tracks/", s.handleAPITracks)
	mux.HandleFunc("/stats", s.handleStats)
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "web/favicon.ico")
	})
	mux.HandleFunc("/logo.png", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "web/logo.png")
	})
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Root fallback: handles /{token} directly
	mux.HandleFunc("/", s.handleRoot)

	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", s.cfg.BindAddress, s.cfg.Port),
		Handler:      enableCORS(panicRecoveryMiddleware(s.rateLimitMiddleware(mux))),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0, // Streaming responses must not have a write timeout
		IdleTimeout:  120 * time.Second,
	}

	log.Printf("[HTTPServer] Listening on %s:%d (FQDN: %s)...", s.cfg.BindAddress, s.cfg.Port, s.cfg.FQDN)

	go func() {
		<-ctx.Done()
		_ = server.Close()
	}()

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func panicRecoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rErr := recover(); rErr != nil {
				log.Printf("[HTTPServer Panic Recovered] %s %s: %v\n%s", r.Method, r.URL.Path, rErr, debug.Stack())
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (s *HTTPServer) rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := s.getClientIP(r)
		now := time.Now().Unix()

		s.rateMu.Lock()
		timestamps := s.requestLog[ip]
		cutoff := now - 1
		var valid []int64
		for _, ts := range timestamps {
			if ts >= cutoff {
				valid = append(valid, ts)
			}
		}

		limit := s.cfg.RateLimitBurst
		if limit <= 0 {
			limit = 20
		}

		if len(valid) >= limit {
			s.requestLog[ip] = valid
			s.rateMu.Unlock()
			w.Header().Set("Retry-After", "1")
			http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
			return
		}

		valid = append(valid, now)
		s.requestLog[ip] = valid
		s.rateMu.Unlock()

		next.ServeHTTP(w, r)
	})
}

func enableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, HEAD, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Range, Authorization, Content-Type, X-Requested-With")
		w.Header().Set("Access-Control-Expose-Headers", "Content-Range, Content-Length, Accept-Ranges, Content-Disposition")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *HTTPServer) getClientIP(r *http.Request) string {
	if ip := r.Header.Get("CF-Connecting-IP"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		parts := strings.Split(ip, ",")
		return strings.TrimSpace(parts[0])
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return r.RemoteAddr
}

// -----------------------------------------------------------------------------
// /watch/{token} Handler: Web Player or On-the-Fly FFmpeg Remux
// -----------------------------------------------------------------------------

func (s *HTTPServer) handleWatch(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.URL.Path, "/watch/")
	token = strings.TrimPrefix(token, "/")
	if token == "" {
		http.Error(w, "Missing token", http.StatusBadRequest)
		return
	}

	// Clean any sub-path (e.g. /watch/{token}/...)
	if parts := strings.SplitN(token, "/", 2); len(parts) > 0 {
		token = parts[0]
	}

	payload, err := crypto.ResolveTokenOrLegacy(token, s.cfg.BinChannel, s.cfg.EncryptionKey)
	if err != nil {
		http.Error(w, "Invalid or expired token", http.StatusNotFound)
		return
	}

	q := queryValues(r.URL.Query())
	audioStr := q.getOrDefault("audio", "-1")
	subStr := q.getOrDefault("sub", "-1")
	ssStr := q.getOrDefault("ss", "0")

	audioIdx, _ := strconv.Atoi(audioStr)
	subIdx, _ := strconv.Atoi(subStr)
	seekSec, _ := strconv.ParseFloat(ssStr, 64)

	// If audio or sub switching or seek is requested, trigger FFmpeg on-the-fly remux
	if audioIdx >= 0 || subIdx >= 0 || seekSec > 0 {
		localStreamURL := fmt.Sprintf("http://127.0.0.1:%d/dl/%s", s.cfg.Port, token)
		err := StreamRemuxWithFFmpeg(r.Context(), w, localStreamURL, audioIdx, subIdx, seekSec, payload.FileName)
		if err != nil {
			log.Printf("[FFmpeg Remux] Error: %v", err)
		}
		return
	}

	// Otherwise, render dark-mode responsive Plyr web player template
	if s.templates != nil {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		data := map[string]interface{}{
			"file_name":  payload.FileName,
			"src":        fmt.Sprintf("/dl/%s", token),
			"tracks_url": fmt.Sprintf("/api/tracks/%s", token),
			"token":      token,
		}
		if err := s.templates.ExecuteTemplate(w, "req.html", data); err == nil {
			return
		}
	}

	// Fallback simple HTML player if template execution fails
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!DOCTYPE html><html><head><title>%s</title><meta name="viewport" content="width=device-width, initial-scale=1"></head>
<body style="margin:0;background:#090a0f;display:flex;align-items:center;justify-content:center;height:100vh;">
<video controls autoplay style="max-width:100%%;max-height:100%%;" src="/dl/%s"></video>
</body></html>`, template.HTMLEscapeString(payload.FileName), token)
}

// -----------------------------------------------------------------------------
// /dl/{token} Handler: High-Performance Binary Range Streamer
// -----------------------------------------------------------------------------

func (s *HTTPServer) handleDownload(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.URL.Path, "/dl/")
	token = strings.TrimPrefix(token, "/")
	if token == "" {
		http.Error(w, "Missing token", http.StatusBadRequest)
		return
	}
	if parts := strings.SplitN(token, "/", 2); len(parts) > 0 {
		token = parts[0]
	}

	s.streamToken(w, r, token)
}

func (s *HTTPServer) handleRoot(w http.ResponseWriter, r *http.Request) {
	raw := strings.TrimPrefix(r.URL.Path, "/")
	if raw == "" {
		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("FileToLink Go Streaming Server Online"))
		return
	}

	token := raw
	if parts := strings.SplitN(raw, "/", 2); len(parts) > 0 {
		token = parts[0]
	}

	s.streamToken(w, r, token)
}

func (s *HTTPServer) streamToken(w http.ResponseWriter, r *http.Request, token string) {
	payload, err := crypto.ResolveTokenOrLegacy(token, s.cfg.BinChannel, s.cfg.EncryptionKey)
	if err != nil {
		http.Error(w, "Invalid or expired token", http.StatusNotFound)
		return
	}

	bot, err := s.pool.GetNextAvailable(nil, int(s.cfg.APIID), s.cfg.APIHash)
	if err != nil {
		http.Error(w, "No streaming sessions available", http.StatusServiceUnavailable)
		return
	}

	// Resolve exact size & filename from Telegram if not in token
	if payload.FileSize <= 0 || payload.FileName == "" || payload.FileName == "download" {
		exactSize, exactName, _, rErr := s.fetcher.ResolveDocumentWithBot(r.Context(), bot, payload.ChatID, payload.MessageID)
		if rErr == nil && exactSize > 0 {
			payload.FileSize = exactSize
			if exactName != "" {
				payload.FileName = exactName
			}
		}
	}

	totalSize := payload.FileSize
	if totalSize <= 0 {
		http.Error(w, "Invalid file size", http.StatusInternalServerError)
		return
	}

	startByte := int64(0)
	endByte := totalSize - 1
	isPartial := false

	rangeHeader := r.Header.Get("Range")
	if rangeHeader != "" && strings.HasPrefix(rangeHeader, "bytes=") {
		parts := strings.Split(strings.TrimPrefix(rangeHeader, "bytes="), "-")
		if len(parts) == 2 {
			if st, pErr := strconv.ParseInt(parts[0], 10, 64); pErr == nil {
				startByte = st
			}
			if en, pErr := strconv.ParseInt(parts[1], 10, 64); pErr == nil && en > 0 {
				endByte = en
			}
			if endByte >= totalSize {
				endByte = totalSize - 1
			}
			if startByte > endByte {
				w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", totalSize))
				http.Error(w, "Range Not Satisfiable", http.StatusRequestedRangeNotSatisfiable)
				return
			}
			isPartial = true
		}
	}

	contentLength := endByte - startByte + 1
	fileName := payload.FileName
	if fileName == "" {
		fileName = "download.bin"
	}

	mimeType := mime.TypeByExtension(filepath.Ext(fileName))
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`,
		strings.ReplaceAll(fileName, `"`, `\"`), url.PathEscape(fileName)))

	if isPartial {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", startByte, endByte, totalSize))
		w.Header().Set("Content-Length", strconv.FormatInt(contentLength, 10))
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.Header().Set("Content-Length", strconv.FormatInt(totalSize, 10))
		w.WriteHeader(http.StatusOK)
	}

	if r.Method == http.MethodHead {
		return
	}

	s.activeDownloads.Add(1)
	s.totalDownloads.Add(1)
	defer s.activeDownloads.Add(-1)

	var streamBytes int64
	trackProgress := func(n int64) {
		streamBytes += n
		s.totalBytesServed.Add(n)
	}

	ctx := r.Context()
	err = s.fetcher.StreamMessageRange(
		ctx,
		s.cfg.APIID,
		s.cfg.APIHash,
		nil,
		payload.ChatID,
		payload.MessageID,
		startByte,
		endByte,
		totalSize,
		w,
		trackProgress,
	)

	if s.database != nil && streamBytes > 0 {
		s.database.RecordStreamTransfer(streamBytes)
	}

	if err != nil && err != context.Canceled {
		log.Printf("[HTTPServer] Stream finished with notice: %v", err)
	}
}

// -----------------------------------------------------------------------------
// Permanent Link Redirection Handler
// -----------------------------------------------------------------------------

func (s *HTTPServer) handlePermanentRedirect(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("path")
	if token == "" {
		token = r.URL.Query().Get("token")
	}
	if token == "" {
		http.Error(w, "Missing token parameter", http.StatusBadRequest)
		return
	}

	target := fmt.Sprintf("%s/watch/%s", s.cfg.BuildEffectiveBaseURL(), token)
	if strings.Contains(r.URL.Path, "dl") || strings.Contains(r.URL.Path, "download") {
		target = fmt.Sprintf("%s/dl/%s", s.cfg.BuildEffectiveBaseURL(), token)
	}

	http.Redirect(w, r, target, http.StatusFound)
}

// -----------------------------------------------------------------------------
// REST API: POST or GET /api/generate_link
// -----------------------------------------------------------------------------

func (s *HTTPServer) handleAPIGenerateLink(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var channelID int64
	var messageID int64

	if r.Method == http.MethodPost {
		var req struct {
			ChannelID int64 `json:"channel_id"`
			ChatID    int64 `json:"chat_id"`
			MessageID int64 `json:"message_id"`
			MsgID     int64 `json:"msg_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err == nil {
			if req.ChannelID != 0 {
				channelID = req.ChannelID
			} else {
				channelID = req.ChatID
			}
			if req.MessageID != 0 {
				messageID = req.MessageID
			} else {
				messageID = req.MsgID
			}
		}
	}

	// Fallback to query params if not found in JSON or for GET requests
	if channelID == 0 {
		chStr := r.URL.Query().Get("channel_id")
		if chStr == "" {
			chStr = r.URL.Query().Get("chat_id")
		}
		channelID, _ = strconv.ParseInt(chStr, 10, 64)
	}
	if messageID == 0 {
		msgStr := r.URL.Query().Get("message_id")
		if msgStr == "" {
			msgStr = r.URL.Query().Get("msg_id")
		}
		messageID, _ = strconv.ParseInt(msgStr, 10, 64)
	}

	if channelID == 0 || messageID == 0 {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "channel_id and message_id are required",
		})
		return
	}

	if s.forwarder == nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "bot forwarder service unavailable",
		})
		return
	}

	downloadURL, streamURL, fileName, fileSize, err := s.forwarder.ForwardAndGenerateLink(r.Context(), channelID, messageID)
	if err != nil {
		log.Printf("[APIGenerateLink] Error generating link for channel %d msg %d: %v", channelID, messageID, err)
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":       true,
		"download_link": downloadURL,
		"stream_link":   streamURL,
		"file_name":     fileName,
		"file_size":     fileSize,
	})
}

// -----------------------------------------------------------------------------
// REST API: GET /api/tracks/{token}
// -----------------------------------------------------------------------------

func (s *HTTPServer) handleAPITracks(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimPrefix(r.URL.Path, "/api/tracks/")
	token = strings.TrimPrefix(token, "/")
	if token == "" {
		http.Error(w, "Missing token", http.StatusBadRequest)
		return
	}

	q := r.URL.Query()
	subStr := q.Get("sub")
	if subStr != "" {
		subIdx, _ := strconv.Atoi(subStr)
		localStreamURL := fmt.Sprintf("http://127.0.0.1:%d/dl/%s", s.cfg.Port, token)
		_ = StreamSubtitleTrack(r.Context(), w, localStreamURL, subIdx, q.Get("format"))
		return
	}

	localStreamURL := fmt.Sprintf("http://127.0.0.1:%d/dl/%s", s.cfg.Port, token)
	tracks, err := ProbeMediaTracks(r.Context(), localStreamURL)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   err.Error(),
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success": true,
		"tracks":  tracks,
	})
}

// -----------------------------------------------------------------------------
// /stats Endpoint: Real-Time Stream & Traffic Tracking
// -----------------------------------------------------------------------------

func (s *HTTPServer) handleStats(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()

	var traffic *db.TrafficStats
	if s.database != nil {
		traffic, _ = s.database.GetTrafficStats(ctx)
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"active_downloads":   s.activeDownloads.Load(),
		"total_downloads":    s.totalDownloads.Load(),
		"total_bytes_served": s.totalBytesServed.Load(),
		"active_sessions":    s.pool.ActiveSessionCount(),
		"traffic_stats":      traffic,
		"uptime_seconds":     time.Now().Unix(),
	})
}

func (s *HTTPServer) handleReelRandom(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	if s.database == nil {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   "Database offline",
		})
		return
	}

	msgID, _ := s.database.GetRandomReelMedia(r.Context())
	if msgID == 0 {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"success": false,
			"error":   "No media found in reel collection",
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"success":    true,
		"message_id": msgID,
	})
}

type queryValues url.Values

func (q queryValues) getOrDefault(key, def string) string {
	v := url.Values(q).Get(key)
	if v == "" {
		return def
	}
	return v
}

// Helper to suppress unused errors
var _ = io.Copy
