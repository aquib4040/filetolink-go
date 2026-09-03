package bot

import (
	"context"
	"log"
	"strings"

	"filetolink-go/internal/pool"

	"github.com/gotd/td/tg"
)

func (bm *BotManager) checkReelMedia(ctx context.Context, msg *tg.Message, peerID int64) {
	if bm.cfg.ReelChannelID == 0 || peerID != pool.FormatChannelID(bm.cfg.ReelChannelID) || msg.Media == nil {
		return
	}

	fileType := "video"
	if _, ok := msg.Media.(*tg.MessageMediaPhoto); ok {
		fileType = "photo"
	}

	_ = bm.database.SaveReelMedia(ctx, msg.ID, fileType)
}

// replyWithReel copies a random reel media from the reel channel and sends it with caption+markup.
// This is the exact same logic as Pyrogram's bot.copy_message() — fetch msg, extract media, send via MessagesSendMedia.
// Falls back to plain text if no reel media is available or on any error.
func (bm *BotManager) replyWithReel(ctx context.Context, peer tg.InputPeerClass, replyToID int, caption string, replyMarkup tg.ReplyMarkupClass) (int, error) {
	if bm.cfg.ReelChannelID == 0 {
		return bm.sendTextWithMarkup(ctx, peer, caption, replyMarkup)
	}

	reelMsgID, _ := bm.database.GetRandomReelMedia(ctx)
	if reelMsgID <= 0 {
		log.Printf("[Reel] No reel media found in DB, falling back to text")
		return bm.sendTextWithMarkup(ctx, peer, caption, replyMarkup)
	}

	// Step 1: Fetch the message from the reel channel (same as Pyrogram's get_messages)
	raw := pool.RawChannelID(bm.cfg.ReelChannelID)
	hash := getCachedChannelAccessHash(raw)
	if hash == 0 {
		hash = bm.ResolveChannelAccessHash(ctx, bm.cfg.ReelChannelID)
	}

	reelChannel := &tg.InputChannel{ChannelID: raw, AccessHash: hash}
	res, err := bm.api.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
		Channel: reelChannel,
		ID:      []tg.InputMessageClass{&tg.InputMessageID{ID: reelMsgID}},
	})
	if err != nil {
		log.Printf("[Reel] ChannelsGetMessages failed for msg %d: %v", reelMsgID, err)
		return bm.sendTextWithMarkup(ctx, peer, caption, replyMarkup)
	}

	msgs := extractMessagesFromClass(res)
	if len(msgs) == 0 || msgs[0].Media == nil {
		log.Printf("[Reel] Msg %d has no media or was empty, removing from DB", reelMsgID)
		_ = bm.database.DeleteReelMedia(ctx, reelMsgID)
		return bm.sendTextWithMarkup(ctx, peer, caption, replyMarkup)
	}

	// Step 2: Extract InputMedia from the fetched message (same as Pyrogram building file_id)
	targetMedia := msgs[0].Media
	var inputMedia tg.InputMediaClass

	switch m := targetMedia.(type) {
	case *tg.MessageMediaDocument:
		if doc, ok := m.Document.(*tg.Document); ok {
			inputMedia = &tg.InputMediaDocument{
				ID: &tg.InputDocument{
					ID:            doc.ID,
					AccessHash:    doc.AccessHash,
					FileReference: doc.FileReference,
				},
			}
		}
	case *tg.MessageMediaPhoto:
		if photo, ok := m.Photo.(*tg.Photo); ok {
			inputMedia = &tg.InputMediaPhoto{
				ID: &tg.InputPhoto{
					ID:            photo.ID,
					AccessHash:    photo.AccessHash,
					FileReference: photo.FileReference,
				},
			}
		}
	}

	if inputMedia == nil {
		log.Printf("[Reel] Could not extract InputMedia from msg %d", reelMsgID)
		return bm.sendTextWithMarkup(ctx, peer, caption, replyMarkup)
	}

	// Step 3: Send the media with caption (same as Pyrogram's send_video/send_photo/send_document)
	plainText, entities := parseHTML(caption)

	// Telegram caption limit is 1024 chars — if exceeded, send media alone then text separately
	if len([]rune(plainText)) > 1024 {
		mediaReq := &tg.MessagesSendMediaRequest{
			Peer:     peer,
			Media:    inputMedia,
			RandomID: getRandomID(),
		}
		if replyToID > 0 {
			mediaReq.ReplyTo = &tg.InputReplyToMessage{ReplyToMsgID: replyToID}
		}
		_, _ = bm.api.MessagesSendMedia(ctx, mediaReq)
		return bm.sendTextWithMarkup(ctx, peer, caption, replyMarkup)
	}

	req := &tg.MessagesSendMediaRequest{
		Peer:        peer,
		Media:       inputMedia,
		Message:     plainText,
		Entities:    entities,
		RandomID:    getRandomID(),
		ReplyMarkup: replyMarkup,
	}
	if replyToID > 0 {
		req.ReplyTo = &tg.InputReplyToMessage{ReplyToMsgID: replyToID}
	}

	sentRes, sendErr := bm.api.MessagesSendMedia(ctx, req)
	if sendErr == nil {
		return extractMsgID(sentRes), nil
	}

	log.Printf("[Reel] MessagesSendMedia failed for msg %d: %v", reelMsgID, sendErr)

	errStr := sendErr.Error()

	// Handle caption too long
	if strings.Contains(errStr, "MEDIA_CAPTION_TOO_LONG") {
		req.Message = ""
		req.Entities = nil
		req.ReplyMarkup = nil
		_, _ = bm.api.MessagesSendMedia(ctx, req)
		return bm.sendTextWithMarkup(ctx, peer, caption, replyMarkup)
	}

	// Handle invalid/deleted message
	if strings.Contains(errStr, "MESSAGE_ID_INVALID") || strings.Contains(errStr, "MEDIA_EMPTY") {
		_ = bm.database.DeleteReelMedia(ctx, reelMsgID)
	}

	// Fallback
	return bm.sendTextWithMarkup(ctx, peer, caption, replyMarkup)
}
