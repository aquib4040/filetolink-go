package bot

import (
	"context"
	"crypto/rand"
	"fmt"
	"log"
	"math/big"
	"strings"
	"sync"
	"time"

	"filetolink-go/internal/config"
	"filetolink-go/internal/db"
	"filetolink-go/internal/markup"
	"filetolink-go/internal/pool"

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
	client     *telegram.Client
	api        *tg.Client
	botUser    *tg.User
	dispatcher tg.UpdateDispatcher
	uptime     time.Time
	mu         sync.RWMutex
}

func NewBotManager(cfg *config.Config, p *pool.SessionPool, d *db.BotDatabase) *BotManager {
	dispatcher := tg.NewUpdateDispatcher()

	bm := &BotManager{
		cfg:        cfg,
		pool:       p,
		database:   d,
		dispatcher: dispatcher,
		uptime:     time.Now(),
	}

	bm.setupHandlers()
	return bm
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

func extractMsgID(updates tg.UpdatesClass) int {
	switch u := updates.(type) {
	case *tg.Updates:
		for _, up := range u.Updates {
			if m, ok := up.(*tg.UpdateNewMessage); ok {
				if msg, ok := m.Message.(*tg.Message); ok {
					return msg.ID
				}
			}
			if m, ok := up.(*tg.UpdateNewChannelMessage); ok {
				if msg, ok := m.Message.(*tg.Message); ok {
					return msg.ID
				}
			}
		}
	case *tg.UpdateShortSentMessage:
		return u.ID
	}
	return 0
}

func toInputPeer(chatID int64) tg.InputPeerClass {
	if chatID > 0 {
		return &tg.InputPeerUser{UserID: chatID}
	}
	raw := pool.RawChannelID(chatID)
	return &tg.InputPeerChannel{ChannelID: raw}
}

func humanBytes(bytes int64) string {
	if bytes <= 0 {
		return "0 B"
	}
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// Suppress unused imports
var _ = markup.NewInlineMarkup
