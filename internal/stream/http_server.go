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

type HTTPServer struct {
	cfg          *config.Config
	pool         *pool.SessionPool
	fetcher      *ParallelFetcher
	database     *db.BotDatabase
	templates    *template.Template
	
	activeDownloads  atomic.Int64
	totalDownloads   atomic.Int64
	totalBytesServed atomic.Int64

	rateMu     sync.Mutex
	requestLog map[string][]int64
}

func NewHTTPServer(cfg *config.Config, p *pool.SessionPool, d *db.BotDatabase) *HTTPServer {
	// Parse HTML templates from web/template
	tmpl, err := template.ParseGlob("web/template/*.html")
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

	// Web Player & Downloads (Stateless token in path, NO filename or hash in link)
	mux.HandleFunc("/watch/", s.handleWatch)
	mux.HandleFunc("/dl/", s.handleDownload)
	mux.HandleFunc("/watch", s.handlePermanentRedirect)
	mux.HandleFunc("/download", s.handlePermanentRedirect)

	// API endpoints
	mux.HandleFunc("/api/generate_link", s.handleAPIGenerateLink)
	mux.HandleFunc("/api/file_stream_url", s.handleAPIFileStreamURL)
	mux.HandleFunc("/api/stream/", s.handleAPIChatStream)
	mux.HandleFunc("/api/tracks/", s.handleAPITracks)
	mux.HandleFunc("/stats", s.handleStats)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Root fallback: handles /{token} directly
	mux.HandleFunc("/", s.handleRoot)

	server := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", s.cfg.BindAddress, s.cfg.Port),
		Handler:      enableCORS(s.rateLimitMiddleware(mux)),
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
		http.Error(w, "Missing path parameter", http.StatusBadRequest)
		return
	}

	target := fmt.Sprintf("%s/watch/%s", s.cfg.BuildBaseURL(), token)
	if strings.Contains(r.URL.Path, "download") {
		target = fmt.Sprintf("%s/dl/%s", s.cfg.BuildBaseURL(), token)
	}

	http.Redirect(w, r, target, http.StatusFound)
}

// -----------------------------------------------------------------------------
// REST API: POST /api/generate_link
// -----------------------------------------------------------------------------

func (s *HTTPServer) handleAPIGenerateLink(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		ChannelID int64 `json:"channel_id"`
		MessageID int64 `json:"message_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.ChannelID == 0 || req.MessageID == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"error":   "channel_id and message_id are required",
		})
		return
	}

	bot, err := s.pool.GetNextAvailable(nil, int(s.cfg.APIID), s.cfg.APIHash)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "bot session unavailable"})
		return
	}

	exactSize, fileName, _, err := s.fetcher.ResolveDocumentWithBot(r.Context(), bot, req.ChannelID, req.MessageID)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": err.Error()})
		return
	}

	payload := &crypto.FileTokenPayload{
		ChatID:    req.ChannelID,
		MessageID: req.MessageID,
		FileHash:  "apigen",
		FileSize:  exactSize,
		FileName:  fileName,
		CreatedAt: time.Now().Unix(),
	}

	token, err := crypto.EncryptPayload(payload, s.cfg.EncryptionKey)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"success": false, "error": "failed to encrypt token"})
		return
	}

	baseURL := s.cfg.BuildEffectiveBaseURL()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":       true,
		"download_link": fmt.Sprintf("%s/dl/%s", baseURL, token),
		"stream_link":   fmt.Sprintf("%s/watch/%s", baseURL, token),
		"file_name":     fileName,
		"file_size":     exactSize,
	})
}

// -----------------------------------------------------------------------------
// REST API: GET /api/file_stream_url
// -----------------------------------------------------------------------------

func (s *HTTPServer) handleAPIFileStreamURL(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	msgIDStr := q.Get("message_id")
	chatIDStr := q.Get("chat_id")
	if msgIDStr == "" || chatIDStr == "" {
		http.Error(w, `{"error":"Missing message_id or chat_id"}`, http.StatusBadRequest)
		return
	}

	msgID, _ := strconv.ParseInt(msgIDStr, 10, 64)
	chatID, _ := strconv.ParseInt(chatIDStr, 10, 64)

	payload := &crypto.FileTokenPayload{
		ChatID:    chatID,
		MessageID: msgID,
		FileHash:  q.Get("hash"),
		CreatedAt: time.Now().Unix(),
	}

	token, err := crypto.EncryptPayload(payload, s.cfg.EncryptionKey)
	if err != nil {
		http.Error(w, `{"error":"encryption error"}`, http.StatusInternalServerError)
		return
	}

	streamURL := fmt.Sprintf("%s/watch/%s", s.cfg.BuildEffectiveBaseURL(), token)
	dlURL := fmt.Sprintf("%s/dl/%s", s.cfg.BuildEffectiveBaseURL(), token)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"success":       true,
		"stream_url":    streamURL,
		"download_link": dlURL,
	})
}

// -----------------------------------------------------------------------------
// REST API: GET /api/stream/{chat_id}/{path}
// -----------------------------------------------------------------------------

func (s *HTTPServer) handleAPIChatStream(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/api/stream/")
	parts := strings.SplitN(trimmed, "/", 2)
	if len(parts) < 2 {
		http.Error(w, "Invalid api stream path", http.StatusBadRequest)
		return
	}

	chatID, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		http.Error(w, "Invalid chat_id", http.StatusBadRequest)
		return
	}

	token := parts[1]
	payload, err := crypto.ResolveTokenOrLegacy(token, chatID, s.cfg.EncryptionKey)
	if err != nil {
		http.Error(w, "Invalid token or path", http.StatusNotFound)
		return
	}
	payload.ChatID = chatID

	s.streamToken(w, r, token)
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
