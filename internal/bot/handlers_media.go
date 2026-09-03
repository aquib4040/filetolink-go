package bot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"strings"
	"time"

	"filetolink-go/internal/crypto"
	"filetolink-go/internal/db"
	"filetolink-go/internal/markup"
	"filetolink-go/internal/pool"

	"github.com/gotd/td/tg"
)

func (bm *BotManager) handleMedia(ctx context.Context, msg *tg.Message, senderID, chatID int64, isPrivate bool) error {
	peer := toInputPeer(chatID)

	// 1. Permission check
	if isPrivate {
		if !bm.isAllowedInPM(ctx, senderID) {
			var rows [][]tg.KeyboardButtonClass
			rows = append(rows, []tg.KeyboardButtonClass{
				markup.NewURLButtonWithStyle(markup.ToSmallCaps("Join Community"), "https://t.me/Anime_Canon", markup.StyleGreen),
			})
			return bm.sendText(ctx, peer, "ʏᴏᴜ ᴀʀᴇ ɴᴏᴛ ᴀ ᴘʀᴇᴍɪᴜᴍ ᴜsᴇʀ.\nʏᴏᴜ ᴄᴀɴ ᴜsᴇ ᴍᴇ ɪɴ ᴀɴ ᴀᴜᴛʜᴏʀɪᴢᴇᴅ ɢʀᴏᴜᴘ!")
		}
	} else {
		// In groups, ignore media unless forwarded/replied with /link
		return nil
	}

	// 2. Start in DM check
	if !isPrivate && !bm.database.HasUserStarted(ctx, senderID) {
		bm.sendStartInDMPrompt(ctx, peer, msg.ID)
		return nil
	}

	// 3. Dynamic FSub check
	if !bm.checkFSub(ctx, senderID, peer) {
		return nil
	}

	// 4. Send initial status
	statusMsgID, err := bm.sendTextWithMarkup(ctx, peer, "⏳ <b>Processing your file...</b>", nil)
	if err != nil {
		return err
	}

	// 5. Forward media to BIN_CHANNEL
	binPeer := toInputPeer(bm.cfg.BinChannel)
	fwdRes, err := bm.api.MessagesForwardMessages(ctx, &tg.MessagesForwardMessagesRequest{
		FromPeer: peer,
		ToPeer:   binPeer,
		ID:       []int{msg.ID},
		RandomID: []int64{getRandomID()},
	})
	if err != nil {
		log.Printf("[Bot] Failed to forward media to BIN_CHANNEL: %v", err)
		_ = bm.editMessage(ctx, peer, statusMsgID, "❌ <b>Failed to process media file.</b>", nil)
		return err
	}

	fwdMsgID := extractMsgID(fwdRes)
	if fwdMsgID == 0 {
		_ = bm.editMessage(ctx, peer, statusMsgID, "❌ <b>Failed to resolve stored message.</b>", nil)
		return fmt.Errorf("forwarded message ID not found")
	}

	// 6. Extract file metadata
	fileName, fileSize, fileHash := extractMediaInfo(msg)

	// 7. Generate stateless encrypted token
	payload := &crypto.FileTokenPayload{
		ChatID:    bm.cfg.BinChannel,
		MessageID: int64(fwdMsgID),
		FileHash:  fileHash,
		FileSize:  fileSize,
		FileName:  fileName,
		CreatedAt: time.Now().Unix(),
	}

	token, err := crypto.EncryptPayload(payload, bm.cfg.EncryptionKey)
	if err != nil {
		_ = bm.editMessage(ctx, peer, statusMsgID, "❌ <b>Encryption error.</b>", nil)
		return err
	}

	// 8. Construct clean URLs (NO file hash and NO file name in URL!)
	baseURL := bm.cfg.BuildEffectiveBaseURL()
	streamURL := fmt.Sprintf("%s/watch/%s", baseURL, token)
	downloadURL := fmt.Sprintf("%s/dl/%s", baseURL, token)

	// 9. Format response text
	text := fmt.Sprintf("✨ <b>Your Links are Ready!</b> ✨\n\n"+
		"📁 <b>File:</b> <code>%s</code>\n"+
		"📦 <b>Size:</b> <code>%s</code>\n\n"+
		"🚀 <b>Download:</b> <code>%s</code>\n"+
		"🖥️ <b>Stream:</b> <code>%s</code>\n\n"+
		"⌛️ <i>Links remain permanently active.</i>",
		htmlEscape(fileName), humanBytes(fileSize), downloadURL, streamURL)

	// 10. Colorful buttons from filestore
	var rows [][]tg.KeyboardButtonClass
	rows = append(rows, []tg.KeyboardButtonClass{
		markup.NewURLButtonWithStyle(markup.ToSmallCaps("Stream"), streamURL, markup.StyleGreen),
		markup.NewURLButtonWithStyle(markup.ToSmallCaps("Download"), downloadURL, markup.StyleBlue),
	})
	rows = append(rows, []tg.KeyboardButtonClass{
		markup.NewCallbackButtonWithStyle(markup.ToSmallCaps("Close"), "close", markup.StyleRed),
	})

	_ = bm.editMessage(ctx, peer, statusMsgID, text, markup.NewInlineMarkup(rows))
	return nil
}

func (bm *BotManager) isAllowedInPM(ctx context.Context, userID int64) bool {
	if userID == bm.cfg.OwnerID {
		return true
	}
	if bm.database.IsPremiumUser(ctx, userID) {
		return true
	}
	return bm.database.GetPMMode(ctx, bm.cfg.PMModeDefault)
}

func (bm *BotManager) sendStartInDMPrompt(ctx context.Context, peer tg.InputPeerClass, replyToID int) {
	botUsername := "bot"
	if bm.botUser != nil && bm.botUser.Username != "" {
		botUsername = bm.botUser.Username
	}
	startURL := fmt.Sprintf("https://t.me/%s?start=start", botUsername)

	var rows [][]tg.KeyboardButtonClass
	rows = append(rows, []tg.KeyboardButtonClass{
		markup.NewURLButtonWithStyle(markup.ToSmallCaps("Start in DM"), startURL, markup.StyleGreen),
	})

	_, _ = bm.sendTextWithMarkup(ctx, peer,
		"⚠️ <b>Please start the bot in private first to use it.</b>",
		markup.NewInlineMarkup(rows))
}

func (bm *BotManager) checkFSub(ctx context.Context, userID int64, peer tg.InputPeerClass) bool {
	channels, err := bm.database.ListFSubChannels(ctx)
	if err != nil || len(channels) == 0 {
		return true
	}

	var missing []db.FSubChannel
	for _, ch := range channels {
		// Check member status
		res, err := bm.api.ChannelsGetParticipant(ctx, &tg.ChannelsGetParticipantRequest{
			Channel:     &tg.InputChannel{ChannelID: pool.RawChannelID(ch.ChannelID)},
			Participant: &tg.InputPeerUser{UserID: userID},
		})
		if err != nil || res == nil {
			missing = append(missing, ch)
		}
	}

	if len(missing) > 0 {
		var rows [][]tg.KeyboardButtonClass
		for _, m := range missing {
			rows = append(rows, []tg.KeyboardButtonClass{
				markup.NewURLButtonWithStyle(markup.ToSmallCaps("Join Channel"), m.InviteURL, markup.StyleGreen),
			})
		}
		_, _ = bm.sendTextWithMarkup(ctx, peer,
			"🔒 <b>You must join our official channel(s) to use this bot!</b>",
			markup.NewInlineMarkup(rows))
		return false
	}

	return true
}

func extractMediaInfo(msg *tg.Message) (fileName string, fileSize int64, fileHash string) {
	fileName = "download.bin"
	fileSize = 0

	if doc, ok := msg.Media.(*tg.MessageMediaDocument); ok {
		if d, ok := doc.Document.(*tg.Document); ok {
			fileSize = d.Size
			for _, attr := range d.Attributes {
				if fn, ok := attr.(*tg.DocumentAttributeFilename); ok {
					fileName = fn.FileName
					break
				}
			}
			h := sha256.Sum256([]byte(fmt.Sprintf("%d:%d", d.ID, d.AccessHash)))
			fileHash = hex.EncodeToString(h[:])[:6]
			return
		}
	}

	h := sha256.Sum256([]byte(fmt.Sprintf("%d", msg.ID)))
	fileHash = hex.EncodeToString(h[:])[:6]
	return
}

func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
