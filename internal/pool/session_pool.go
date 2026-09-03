package pool

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
)

const (
	AuthTimeout = 30 * time.Second // Generous timeout for MTProto DH key exchange
)

func isFatalTokenError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "401") ||
		strings.Contains(s, "AUTH_KEY_UNREGISTERED") ||
		strings.Contains(s, "TOKEN_INVALID") ||
		strings.Contains(s, "USER_DEACTIVATED") ||
		strings.Contains(s, "BOT_METHOD_INVALID")
}

// BotSession represents a single authenticated gotd MTProto bot session.
type BotSession struct {
	Index           int
	Token           string
	client          *telegram.Client
	API             *tg.Client
	ctx             context.Context
	cancel          context.CancelFunc
	ready           chan struct{}
	readyErr        error
	lastUsed        atomic.Int64
	cooling         atomic.Int64
	ActiveDownloads atomic.Int32
	TotalDownloads  atomic.Int64
	accessCache     sync.Map // int64 (raw channel ID) → int64 (access hash)
	pool            *SessionPool
}

func (bot *BotSession) StartDownload() {
	bot.ActiveDownloads.Add(1)
	bot.TotalDownloads.Add(1)
	bot.Touch()
}

func (bot *BotSession) EndDownload() {
	if count := bot.ActiveDownloads.Add(-1); count < 0 {
		bot.ActiveDownloads.Store(0)
	}
}

func (bot *BotSession) Touch() {
	bot.lastUsed.Store(time.Now().Unix())
}

// SessionPool manages MTProto bot sessions with:
//   - Non-blocking parallel startup authentication
//   - Fast failure detection with in-memory invalid token blacklisting
//   - Round-robin load balancing across worker bots
type SessionPool struct {
	mu            sync.RWMutex
	sessions      []*BotSession
	tokenMap      map[string]*BotSession
	invalidTokens sync.Map // token (string) -> true
	baseDataPath  string
	rrIndex       int
}

func NewSessionPool(baseDataPath string) *SessionPool {
	return &SessionPool{
		sessions:     make([]*BotSession, 0),
		tokenMap:     make(map[string]*BotSession),
		baseDataPath: baseDataPath,
	}
}

// FormatChannelID ensures channel ID starts with -100
func FormatChannelID(id int64) int64 {
	if id > 0 {
		s := fmt.Sprintf("%d", id)
		if !strings.HasPrefix(s, "100") {
			s = "100" + s
		}
		if parsed, err := strconv.ParseInt("-"+s, 10, 64); err == nil {
			return parsed
		}
	}
	return id
}

// RawChannelID extracts the raw Telegram channel ID from -100XXXXXXXXX format
func RawChannelID(chatID int64) int64 {
	if chatID < 0 {
		s := strconv.FormatInt(-chatID, 10)
		if strings.HasPrefix(s, "100") && len(s) > 3 {
			raw, err := strconv.ParseInt(s[3:], 10, 64)
			if err == nil {
				return raw
			}
		}
	}
	return chatID
}

// IsTokenInvalid checks whether a bot token has been blacklisted in memory
func (p *SessionPool) IsTokenInvalid(token string) bool {
	_, bad := p.invalidTokens.Load(token)
	return bad
}

// MarkTokenInvalid flags a token as invalid in memory so it is skipped immediately
func (p *SessionPool) MarkTokenInvalid(token string, reason string) {
	safeSuffix := token
	if len(safeSuffix) > 6 {
		safeSuffix = safeSuffix[len(safeSuffix)-6:]
	}
	p.invalidTokens.Store(token, true)
	log.Printf("[SessionPool] Token ...%s flagged as INVALID in-memory: %s (will be skipped)", safeSuffix, reason)

	p.mu.Lock()
	defer p.mu.Unlock()
	if bot, exists := p.tokenMap[token]; exists {
		bot.cancel()
		delete(p.tokenMap, token)
		var kept []*BotSession
		for _, s := range p.sessions {
			if s.Token != token {
				kept = append(kept, s)
			}
		}
		p.sessions = kept
	}
}

// InitSession authenticates a bot session. If authentication fails or times out,
// it marks the token as invalid in memory and skips it.
func (p *SessionPool) InitSession(apiID int, apiHash string, token string) (*BotSession, error) {
	if p.IsTokenInvalid(token) {
		return nil, fmt.Errorf("token is blacklisted as invalid in memory")
	}

	p.mu.Lock()
	if bot, exists := p.tokenMap[token]; exists {
		p.mu.Unlock()
		bot.lastUsed.Store(time.Now().Unix())
		select {
		case <-bot.ready:
			if bot.readyErr != nil {
				return nil, bot.readyErr
			}
			return bot, nil
		case <-time.After(AuthTimeout):
			return nil, fmt.Errorf("timeout waiting for session authentication")
		}
	}

	hash := md5.Sum([]byte(token))
	sessionDir := filepath.Join(p.baseDataPath, "session_"+hex.EncodeToString(hash[:]))
	_ = os.MkdirAll(sessionDir, 0755)

	safeSuffix := token
	if len(safeSuffix) > 6 {
		safeSuffix = safeSuffix[len(safeSuffix)-6:]
	}

	client := telegram.NewClient(apiID, apiHash, telegram.Options{
		SessionStorage: &session.FileStorage{
			Path: filepath.Join(sessionDir, "session.json"),
		},
	})

	ctx, cancel := context.WithCancel(context.Background())
	bot := &BotSession{
		Index:  len(p.sessions),
		Token:  token,
		client: client,
		ctx:    ctx,
		cancel: cancel,
		ready:  make(chan struct{}),
		pool:   p,
	}
	bot.lastUsed.Store(time.Now().Unix())

	p.sessions = append(p.sessions, bot)
	p.tokenMap[token] = bot
	p.mu.Unlock()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			err := client.Run(ctx, func(runCtx context.Context) error {
				status, err := client.Auth().Status(runCtx)
				if err != nil {
					bot.readyErr = fmt.Errorf("auth status check failed: %w", err)
					select {
					case <-bot.ready:
					default:
						close(bot.ready)
					}
					return bot.readyErr
				}

				if !status.Authorized {
					if _, err := client.Auth().Bot(runCtx, token); err != nil {
						bot.readyErr = fmt.Errorf("bot auth failed: %w", err)
						select {
						case <-bot.ready:
						default:
							close(bot.ready)
						}
						return bot.readyErr
					}
				}

				bot.API = client.API()
				log.Printf("[SessionPool] Session authenticated: token ...%s", safeSuffix)

				select {
				case <-bot.ready:
				default:
					close(bot.ready)
				}

				<-runCtx.Done()
				return runCtx.Err()
			})

			if err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				// If authorization is revoked / token is invalid, mark immediately and do not loop
				if isFatalTokenError(err) {
					p.MarkTokenInvalid(token, err.Error())
					return
				}

				select {
				case <-ctx.Done():
					return
				case <-time.After(3 * time.Second):
				}
			} else {
				return
			}
		}
	}()

	select {
	case <-bot.ready:
		if bot.readyErr != nil {
			if isFatalTokenError(bot.readyErr) {
				p.MarkTokenInvalid(token, bot.readyErr.Error())
			}
			return nil, bot.readyErr
		}
		return bot, nil
	case <-time.After(AuthTimeout):
		// Do NOT mark token as invalid on timeout! MTProto handshake will complete in background.
		log.Printf("[SessionPool] Notice: Token ...%s handshake still in progress (exceeded %v), continuing in background", safeSuffix, AuthTimeout)
		return nil, fmt.Errorf("timeout waiting for bot auth (token ...%s)", safeSuffix)
	}
}

func (bot *BotSession) ResolveChannel(ctx context.Context, chatID int64) (*tg.InputChannel, error) {
	rawID := RawChannelID(chatID)

	if hash, ok := bot.accessCache.Load(rawID); ok {
		return &tg.InputChannel{
			ChannelID:  rawID,
			AccessHash: hash.(int64),
		}, nil
	}

	if res, err := bot.API.MessagesGetChats(ctx, []int64{rawID}); err == nil && res != nil {
		var chats []tg.ChatClass
		switch c := res.(type) {
		case *tg.MessagesChats:
			chats = c.Chats
		case *tg.MessagesChatsSlice:
			chats = c.Chats
		}
		for _, chat := range chats {
			if ch, ok := chat.(*tg.Channel); ok && ch.ID == rawID && !ch.Min && ch.AccessHash != 0 {
				bot.accessCache.Store(rawID, ch.AccessHash)
				return &tg.InputChannel{
					ChannelID:  rawID,
					AccessHash: ch.AccessHash,
				}, nil
			}
		}
	}

	if res, err := bot.API.ChannelsGetChannels(ctx, []tg.InputChannelClass{
		&tg.InputChannel{
			ChannelID:  rawID,
			AccessHash: 0,
		},
	}); err == nil && res != nil {
		var chats []tg.ChatClass
		switch c := res.(type) {
		case *tg.MessagesChats:
			chats = c.Chats
		case *tg.MessagesChatsSlice:
			chats = c.Chats
		}
		for _, chat := range chats {
			if ch, ok := chat.(*tg.Channel); ok && ch.ID == rawID && ch.AccessHash != 0 {
				bot.accessCache.Store(rawID, ch.AccessHash)
				return &tg.InputChannel{
					ChannelID:  rawID,
					AccessHash: ch.AccessHash,
				}, nil
			}
		}
	}

	return nil, fmt.Errorf("could not resolve channel %d access_hash", chatID)
}

// GetNextAvailable picks a ready session from the provided tokens or pool round-robin,
// skipping any invalid or blacklisted tokens.
func (p *SessionPool) GetNextAvailable(tokens []string, apiID int, apiHash string) (*BotSession, error) {
	// Filter out invalid tokens
	var validTokens []string
	for _, t := range tokens {
		if !p.IsTokenInvalid(t) {
			validTokens = append(validTokens, t)
		}
	}

	p.mu.Lock()
	now := time.Now().Unix()

	var candidates []*BotSession
	if len(validTokens) > 0 {
		for _, token := range validTokens {
			if bot, exists := p.tokenMap[token]; exists {
				select {
				case <-bot.ready:
					if bot.readyErr == nil {
						candidates = append(candidates, bot)
					}
				default:
				}
			}
		}
	} else {
		for _, bot := range p.sessions {
			if !p.IsTokenInvalid(bot.Token) {
				select {
				case <-bot.ready:
					if bot.readyErr == nil {
						candidates = append(candidates, bot)
					}
				default:
				}
			}
		}
	}

	if len(candidates) > 0 {
		p.rrIndex = (p.rrIndex + 1) % len(candidates)
		startIdx := p.rrIndex
		p.mu.Unlock()

		for i := 0; i < len(candidates); i++ {
			idx := (startIdx + i) % len(candidates)
			bot := candidates[idx]
			if now >= bot.cooling.Load() && bot.ActiveDownloads.Load() < 16 {
				bot.lastUsed.Store(now)
				return bot, nil
			}
		}

		// Fallback to least loaded candidate
		best := candidates[0]
		for _, b := range candidates[1:] {
			if b.ActiveDownloads.Load() < best.ActiveDownloads.Load() {
				best = b
			}
		}
		best.lastUsed.Store(now)
		return best, nil
	}

	// If no candidate is currently ready, wait up to 5s on any session currently authenticating
	for _, bot := range p.sessions {
		if !p.IsTokenInvalid(bot.Token) {
			p.mu.Unlock()
			select {
			case <-bot.ready:
				if bot.readyErr == nil {
					bot.lastUsed.Store(time.Now().Unix())
					return bot, nil
				}
			case <-time.After(5 * time.Second):
			}
			p.mu.Lock()
		}
	}

	// If no sessions exist yet, attempt on-demand initialization of the first valid token
	if len(validTokens) > 0 {
		p.mu.Unlock()
		bot, err := p.InitSession(apiID, apiHash, validTokens[0])
		if err == nil && bot != nil {
			return bot, nil
		}
		p.mu.Lock()
	}

	p.mu.Unlock()
	return nil, fmt.Errorf("no ready gotd bot sessions available in pool")
}

// WarmupTokens starts background authentication of multi-worker tokens with controlled concurrency
func (p *SessionPool) WarmupTokens(apiID int, apiHash string, tokens []string) {
	if len(tokens) == 0 {
		return
	}
	log.Printf("[Pool] Starting background warmup for %d worker bot tokens...", len(tokens))

	go func() {
		// Limit to 3 concurrent handshakes to prevent DC connection throttling
		sem := make(chan struct{}, 3)
		var wg sync.WaitGroup

		for _, tok := range tokens {
			tok = strings.TrimSpace(tok)
			if tok == "" || p.IsTokenInvalid(tok) {
				continue
			}

			wg.Add(1)
			go func(t string) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()

				// Slight stagger between connection handshakes
				time.Sleep(250 * time.Millisecond)

				_, _ = p.InitSession(apiID, apiHash, t)
			}(tok)
		}

		wg.Wait()
		log.Printf("[Pool] Multi-token startup complete. %d active sessions in rotation.", p.ActiveSessionCount())
	}()
}

// ActiveSessionCount returns the count of currently connected, authenticated sessions
func (p *SessionPool) ActiveSessionCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	count := 0
	for _, s := range p.sessions {
		if !p.IsTokenInvalid(s.Token) {
			select {
			case <-s.ready:
				if s.readyErr == nil {
					count++
				}
			default:
			}
		}
	}
	return count
}

func (p *SessionPool) StopAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, bot := range p.sessions {
		bot.cancel()
	}
	p.sessions = nil
	p.tokenMap = make(map[string]*BotSession)
}

// SessionWorkload captures realtime per-bot worker streaming load
type SessionWorkload struct {
	Index         int
	TokenSuffix   string
	ActiveStreams int32
	TotalStreams  int64
	IsReady       bool
	IsInvalid     bool
}

// GetWorkloads returns current stream connections and lifetime stats per bot token
func (p *SessionPool) GetWorkloads() []SessionWorkload {
	p.mu.RLock()
	defer p.mu.RUnlock()

	var out []SessionWorkload
	for _, bot := range p.sessions {
		safeSuffix := bot.Token
		if len(safeSuffix) > 6 {
			safeSuffix = safeSuffix[len(safeSuffix)-6:]
		}
		isReady := false
		select {
		case <-bot.ready:
			isReady = bot.readyErr == nil
		default:
		}

		out = append(out, SessionWorkload{
			Index:         bot.Index,
			TokenSuffix:   safeSuffix,
			ActiveStreams: bot.ActiveDownloads.Load(),
			TotalStreams:  bot.TotalDownloads.Load(),
			IsReady:       isReady,
			IsInvalid:     p.IsTokenInvalid(bot.Token),
		})
	}
	return out
}
