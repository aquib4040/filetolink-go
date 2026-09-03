package bot

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"filetolink-go/internal/config"
	"filetolink-go/internal/db"
	"filetolink-go/internal/markup"
	"filetolink-go/internal/pool"
	"filetolink-go/internal/shortener"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/message/entity"
	"github.com/gotd/td/telegram/message/html"
	"github.com/gotd/td/telegram/message/styling"
	"github.com/gotd/td/tg"
)

type BotManager struct {
	cfg        *config.Config
	pool       *pool.SessionPool
	database   *db.BotDatabase
	shortener  *shortener.Shortener
	client     *telegram.Client
	api        *tg.Client
	botUser     *tg.User
	dispatcher  tg.UpdateDispatcher
	uptime      time.Time
	dynSettings db.DynamicBotSettings
	settingsMu  sync.RWMutex
	mu          sync.RWMutex
}

func NewBotManager(cfg *config.Config, p *pool.SessionPool, d *db.BotDatabase) *BotManager {
	dispatcher := tg.NewUpdateDispatcher()

	initialSettings := db.DynamicBotSettings{
		ShortenMediaLinks: cfg.ShortenMediaLinks,
		TokenEnabled:      cfg.TokenEnabled,
		TokenTTLHours:     cfg.TokenTTLHours,
		ShortenerSite:     cfg.ShortenerSite,
		ShortenerAPIKey:   cfg.ShortenerAPIKey,
		PMMode:            cfg.PMModeDefault,
		Batch:             cfg.Batch,
	}

	if d != nil {
		initialSettings = d.GetDynamicSettings(context.Background(), initialSettings)
	}

	bm := &BotManager{
		cfg:         cfg,
		pool:        p,
		database:    d,
		shortener:   shortener.NewURLShortener(initialSettings.ShortenerSite, initialSettings.ShortenerAPIKey, initialSettings.ShortenMediaLinks),
		dispatcher:  dispatcher,
		uptime:      time.Now(),
		dynSettings: initialSettings,
	}

	bm.setupHandlers()
	return bm
}

func (bm *BotManager) GetSettings() db.DynamicBotSettings {
	bm.settingsMu.RLock()
	defer bm.settingsMu.RUnlock()
	return bm.dynSettings
}

func (bm *BotManager) UpdateSetting(ctx context.Context, field string, val any) error {
	bm.settingsMu.Lock()
	defer bm.settingsMu.Unlock()

	switch field {
	case "shorten_media_links":
		if v, ok := val.(bool); ok {
			bm.dynSettings.ShortenMediaLinks = v
			bm.shortener.UpdateConfig(bm.dynSettings.ShortenerSite, bm.dynSettings.ShortenerAPIKey, v)
		}
	case "token_enabled":
		if v, ok := val.(bool); ok {
			bm.dynSettings.TokenEnabled = v
		}
	case "token_ttl_hours":
		if v, ok := val.(int); ok {
			bm.dynSettings.TokenTTLHours = v
		}
	case "shortener_site":
		if v, ok := val.(string); ok {
			bm.dynSettings.ShortenerSite = v
			bm.shortener.UpdateConfig(v, bm.dynSettings.ShortenerAPIKey, bm.dynSettings.ShortenMediaLinks)
		}
	case "shortener_api_key":
		if v, ok := val.(string); ok {
			bm.dynSettings.ShortenerAPIKey = v
			bm.shortener.UpdateConfig(bm.dynSettings.ShortenerSite, v, bm.dynSettings.ShortenMediaLinks)
		}
	case "pm_mode":
		if v, ok := val.(bool); ok {
			bm.dynSettings.PMMode = v
		}
	case "batch":
		if v, ok := val.(bool); ok {
			bm.dynSettings.Batch = v
		}
	}

	if bm.database != nil {
		return bm.database.SetDynamicSetting(ctx, field, val)
	}
	return nil
}

func (bm *BotManager) Start(ctx context.Context) error {
	client := telegram.NewClient(int(bm.cfg.APIID), bm.cfg.APIHash, telegram.Options{
		UpdateHandler: bm.dispatcher,
	})
	bm.client = client

	errChan := make(chan error, 1)

	go func() {
		err := client.Run(ctx, func(runCtx context.Context) error {
			status, err := client.Auth().Status(runCtx)
			if err != nil {
				return fmt.Errorf("bot auth status failed: %w", err)
			}

			if !status.Authorized {
				if _, err := client.Auth().Bot(runCtx, bm.cfg.BotToken); err != nil {
					return fmt.Errorf("bot login failed: %w", err)
				}
			}

			bm.api = client.API()
			self, err := bm.api.UsersGetUsers(runCtx, []tg.InputUserClass{&tg.InputUserSelf{}})
			if err == nil && len(self) > 0 {
				if u, ok := self[0].(*tg.User); ok {
					bm.botUser = u
					log.Printf("[Bot] Logged in as @%s (ID: %d)", u.Username, u.ID)
				}
			}

			// Register official bot commands with Telegram (Admin in bracket for admin tools)
			_ = bm.registerBotCommands(runCtx)

			// Notify and complete previous restart if pending
			bm.checkAndCompleteRestart(runCtx)

			// Start dyno keepalive ping loop if configured
			go bm.startKeepalive(ctx)

			errChan <- nil
			<-runCtx.Done()
			return runCtx.Err()
		})
		if err != nil {
			errChan <- err
		}
	}()

	select {
	case err := <-errChan:
		return err
	case <-time.After(15 * time.Second):
		return fmt.Errorf("timeout waiting for bot startup")
	}
}

func (bm *BotManager) registerBotCommands(ctx context.Context) error {
	commands := []tg.BotCommand{
		{Command: "start", Description: "Start the bot & register user"},
		{Command: "help", Description: "Help and command guide"},
		{Command: "ping", Description: "Check bot latency and server status"},
		{Command: "link", Description: "Generate streaming & download links in groups"},
	}

	if bm.cfg.Batch {
		commands = append(commands, tg.BotCommand{Command: "batch", Description: "Process a batch of consecutive files"})
	}

	adminCommands := []tg.BotCommand{
		{Command: "stats", Description: "(Admin) View bandwidth and traffic stats"},
		{Command: "speedtest", Description: "(Admin) Run network speed test"},
		{Command: "fsub", Description: "(Admin) Force-Sub settings and channels"},
		{Command: "ban", Description: "(Admin) Ban a user from bot"},
		{Command: "unban", Description: "(Admin) Unban a user"},
		{Command: "auth_gc", Description: "(Admin) Authorize a group chat"},
		{Command: "deauth_gc", Description: "(Admin) Deauthorize a group chat"},
		{Command: "listauth_gc", Description: "(Admin) List authorized group chats"},
		{Command: "addpaid", Description: "(Admin) Add paid subscription (e.g. 30d, 1m, 3600s)"},
		{Command: "removepaid", Description: "(Admin) Remove paid user subscription"},
		{Command: "listpaid", Description: "(Admin) List active paid users"},
		{Command: "pmmode", Description: "(Admin) Toggle PM mode on or off"},
		{Command: "restart", Description: "(Admin) Restart bot service"},
	}

	commands = append(commands, adminCommands...)

	_, err := bm.api.BotsSetBotCommands(ctx, &tg.BotsSetBotCommandsRequest{
		Scope:    &tg.BotCommandScopeDefault{},
		LangCode: "en",
		Commands: commands,
	})
	return err
}

func (bm *BotManager) checkAndCompleteRestart(ctx context.Context) {
	if bm.database == nil {
		return
	}
	msgID, chatID, err := bm.database.GetRestartMessage(ctx)
	if err == nil && msgID > 0 && chatID != 0 {
		peer := toInputPeer(chatID)
		_ = bm.editMessage(ctx, peer, int(msgID), "✅ <b>Restart Successful!</b>", nil)
		_ = bm.database.DeleteRestartMessage(ctx)
	}
}

func (bm *BotManager) startKeepalive(ctx context.Context) {
	if bm.cfg.FQDN == "" || strings.Contains(bm.cfg.FQDN, "localhost") {
		return
	}
	interval := bm.cfg.PingInterval
	if interval < 1*time.Minute {
		interval = 10 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	healthURL := fmt.Sprintf("%s/health", bm.cfg.BuildBaseURL())
	client := &http.Client{Timeout: 10 * time.Second}

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			resp, err := client.Get(healthURL)
			if err == nil {
				_ = resp.Body.Close()
			}
		}
	}
}

func (bm *BotManager) setupHandlers() {
	bm.dispatcher.OnNewMessage(func(ctx context.Context, e tg.Entities, u *tg.UpdateNewMessage) error {
		msg, ok := u.Message.(*tg.Message)
		if !ok || msg.Out {
			return nil
		}
		return bm.routeMessage(ctx, msg)
	})

	bm.dispatcher.OnNewChannelMessage(func(ctx context.Context, e tg.Entities, u *tg.UpdateNewChannelMessage) error {
		msg, ok := u.Message.(*tg.Message)
		if !ok || msg.Out {
			return nil
		}
		return bm.routeMessage(ctx, msg)
	})

	bm.dispatcher.OnBotCallbackQuery(func(ctx context.Context, e tg.Entities, u *tg.UpdateBotCallbackQuery) error {
		data := string(u.Data)
		if data == "close" {
			peer := &tg.InputPeerUser{UserID: u.UserID}
			_ = bm.deleteMessages(ctx, peer, []int{u.MsgID})
			_, _ = bm.api.MessagesSetBotCallbackAnswer(ctx, &tg.MessagesSetBotCallbackAnswerRequest{
				QueryID: u.QueryID,
				Message: "Closed",
			})
			return nil
		}

		if u.UserID == bm.cfg.OwnerID {
			peer := &tg.InputPeerUser{UserID: u.UserID}
			switch data {
			case "set_toggle_sml":
				cur := bm.GetSettings().ShortenMediaLinks
				_ = bm.UpdateSetting(ctx, "shorten_media_links", !cur)
				_ = bm.sendMainSettingsPanel(ctx, peer, u.MsgID)
				_, _ = bm.api.MessagesSetBotCallbackAnswer(ctx, &tg.MessagesSetBotCallbackAnswerRequest{QueryID: u.QueryID, Message: "Updated"})
				return nil
			case "set_toggle_token":
				cur := bm.GetSettings().TokenEnabled
				_ = bm.UpdateSetting(ctx, "token_enabled", !cur)
				_ = bm.sendMainSettingsPanel(ctx, peer, u.MsgID)
				_, _ = bm.api.MessagesSetBotCallbackAnswer(ctx, &tg.MessagesSetBotCallbackAnswerRequest{QueryID: u.QueryID, Message: "Updated"})
				return nil
			case "set_ttl_step":
				cur := bm.GetSettings().TokenTTLHours
				next := 12
				switch cur {
				case 6:
					next = 12
				case 12:
					next = 24
				case 24:
					next = 48
				case 48:
					next = 6
				default:
					next = 24
				}
				_ = bm.UpdateSetting(ctx, "token_ttl_hours", next)
				_ = bm.sendMainSettingsPanel(ctx, peer, u.MsgID)
				_, _ = bm.api.MessagesSetBotCallbackAnswer(ctx, &tg.MessagesSetBotCallbackAnswerRequest{QueryID: u.QueryID, Message: fmt.Sprintf("TTL: %dh", next)})
				return nil
			case "set_toggle_pm":
				cur := bm.GetSettings().PMMode
				_ = bm.UpdateSetting(ctx, "pm_mode", !cur)
				_ = bm.sendMainSettingsPanel(ctx, peer, u.MsgID)
				_, _ = bm.api.MessagesSetBotCallbackAnswer(ctx, &tg.MessagesSetBotCallbackAnswerRequest{QueryID: u.QueryID, Message: "Updated"})
				return nil
			case "set_toggle_batch":
				cur := bm.GetSettings().Batch
				_ = bm.UpdateSetting(ctx, "batch", !cur)
				_ = bm.sendMainSettingsPanel(ctx, peer, u.MsgID)
				_, _ = bm.api.MessagesSetBotCallbackAnswer(ctx, &tg.MessagesSetBotCallbackAnswerRequest{QueryID: u.QueryID, Message: "Updated"})
				return nil
			case "set_fsub_menu":
				_ = bm.sendFSubSettingsPanel(ctx, peer)
				_, _ = bm.api.MessagesSetBotCallbackAnswer(ctx, &tg.MessagesSetBotCallbackAnswerRequest{QueryID: u.QueryID, Message: "FSub Panel"})
				return nil
			case "set_refresh":
				_ = bm.sendMainSettingsPanel(ctx, peer, u.MsgID)
				_, _ = bm.api.MessagesSetBotCallbackAnswer(ctx, &tg.MessagesSetBotCallbackAnswerRequest{QueryID: u.QueryID, Message: "Refreshed"})
				return nil
			}
		}

		if strings.HasPrefix(data, "fsub_rm_") {
			chIDStr := strings.TrimPrefix(data, "fsub_rm_")
			if chID, err := strconv.ParseInt(chIDStr, 10, 64); err == nil {
				_ = bm.database.RemoveFSubChannel(ctx, chID)
				peer := &tg.InputPeerUser{UserID: u.UserID}
				_ = bm.sendFSubSettingsPanel(ctx, peer)
				_, _ = bm.api.MessagesSetBotCallbackAnswer(ctx, &tg.MessagesSetBotCallbackAnswerRequest{
					QueryID: u.QueryID,
					Message: "Channel removed",
				})
			}
			return nil
		}
		return nil
	})
}

func (bm *BotManager) routeMessage(ctx context.Context, msg *tg.Message) error {
	senderID := int64(0)
	if msg.FromID != nil {
		if u, ok := msg.FromID.(*tg.PeerUser); ok {
			senderID = u.UserID
		}
	}

	peerID := int64(0)
	isPrivate := false
	switch p := msg.PeerID.(type) {
	case *tg.PeerUser:
		peerID = p.UserID
		isPrivate = true
	case *tg.PeerChat:
		peerID = -p.ChatID
	case *tg.PeerChannel:
		peerID = pool.FormatChannelID(p.ChannelID)
	}

	if senderID == 0 && isPrivate {
		senderID = peerID
	}

	// Banned User Check
	if senderID != 0 && bm.database.IsBanned(ctx, senderID) {
		peer := toInputPeer(peerID)
		_ = bm.sendText(ctx, peer, "⛔ <b>You are banned from using this bot.</b>")
		return nil
	}

	// Reel Channel Monitor (personal tracking)
	bm.checkReelMedia(ctx, msg, peerID)

	text := strings.TrimSpace(msg.Message)

	// If command
	if strings.HasPrefix(text, "/") {
		parts := strings.Fields(text)
		cmd := parts[0]
		if atIdx := strings.Index(cmd, "@"); atIdx != -1 {
			cmd = cmd[:atIdx]
		}
		args := parts[1:]
		return bm.handleCommand(ctx, cmd, args, senderID, peerID, isPrivate, msg)
	}

	// If media in PM or channel
	if msg.Media != nil {
		return bm.handleMedia(ctx, msg, senderID, peerID, isPrivate)
	}

	return nil
}

// -----------------------------------------------------------------------------
// Message Helpers
// -----------------------------------------------------------------------------

func parseHTML(text string) (string, []tg.MessageEntityClass) {
	b := &entity.Builder{}
	styling.Perform(b, html.String(nil, text))
	return b.Complete()
}

func getRandomID() int64 {
	n, err := rand.Int(rand.Reader, big.NewInt(1<<62))
	if err != nil {
		return time.Now().UnixNano()
	}
	return n.Int64()
}

func (bm *BotManager) sendText(ctx context.Context, peer tg.InputPeerClass, text string) error {
	plainText, entities := parseHTML(text)
	_, err := bm.api.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
		Peer:      peer,
		Message:   plainText,
		Entities:  entities,
		NoWebpage: true,
		RandomID:  getRandomID(),
	})
	return err
}

func (bm *BotManager) sendTextWithMarkup(ctx context.Context, peer tg.InputPeerClass, text string, markup tg.ReplyMarkupClass) (int, error) {
	plainText, entities := parseHTML(text)
	updates, err := bm.api.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
		Peer:        peer,
		Message:     plainText,
		Entities:    entities,
		ReplyMarkup: markup,
		NoWebpage:   true,
		RandomID:    getRandomID(),
	})
	if err != nil {
		return 0, err
	}
	return extractMsgID(updates), nil
}

func (bm *BotManager) editMessage(ctx context.Context, peer tg.InputPeerClass, msgID int, text string, markup tg.ReplyMarkupClass) error {
	plainText, entities := parseHTML(text)
	_, err := bm.api.MessagesEditMessage(ctx, &tg.MessagesEditMessageRequest{
		Peer:        peer,
		ID:          msgID,
		Message:     plainText,
		Entities:    entities,
		ReplyMarkup: markup,
		NoWebpage:   true,
	})
	return err
}

func (bm *BotManager) deleteMessages(ctx context.Context, peer tg.InputPeerClass, msgIDs []int) error {
	var valid []int
	for _, id := range msgIDs {
		if id > 0 {
			valid = append(valid, id)
		}
	}
	if len(valid) == 0 {
		return nil
	}
	_, err := bm.api.MessagesDeleteMessages(ctx, &tg.MessagesDeleteMessagesRequest{
		ID:     valid,
		Revoke: true,
	})
	return err
}

func toInputPeer(chatID int64) tg.InputPeerClass {
	if chatID > 0 {
		return &tg.InputPeerUser{UserID: chatID}
	}
	raw := pool.RawChannelID(chatID)
	return &tg.InputPeerChannel{ChannelID: raw}
}

// Suppress unused imports
var _ = markup.NewInlineMarkup
