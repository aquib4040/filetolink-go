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

// replyWithReel sends a random reel media with caption and markup, falling back to text
func (bm *BotManager) replyWithReel(ctx context.Context, peer tg.InputPeerClass, replyToID int, caption string, replyMarkup tg.ReplyMarkupClass) (int, error) {
	if bm.cfg.ReelChannelID != 0 {
		reelMsgID, _ := bm.database.GetRandomReelMedia(ctx)
		if reelMsgID > 0 {
			raw := pool.RawChannelID(bm.cfg.ReelChannelID)
			hash := bm.ResolveChannelAccessHash(ctx, bm.cfg.ReelChannelID)
			reelChannel := &tg.InputChannel{ChannelID: raw, AccessHash: hash}
			res, err := bm.api.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
				Channel: reelChannel,
				ID:      []tg.InputMessageClass{&tg.InputMessageID{ID: reelMsgID}},
			})
			if err != nil {
				log.Printf("[Reel] ChannelsGetMessages failed for msg %d (channel %d, hash %d): %v", reelMsgID, raw, hash, err)
			} else if res != nil {
				msgs := extractMessagesFromClass(res)
				if len(msgs) > 0 && msgs[0].Media != nil {
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

					if inputMedia != nil {
						plainText, entities := parseHTML(caption)
						if len([]rune(plainText)) > 1024 {
							// Caption exceeds Telegram 1024 limit: send media first, then full text
							req := &tg.MessagesSendMediaRequest{
								Peer:     peer,
								Media:    inputMedia,
								RandomID: getRandomID(),
							}
							if replyToID > 0 {
								req.ReplyTo = &tg.InputReplyToMessage{ReplyToMsgID: replyToID}
							}
							_, _ = bm.api.MessagesSendMedia(ctx, req)
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
						log.Printf("[Reel] MessagesSendMedia error for msg %d: %v", reelMsgID, sendErr)
						if strings.Contains(sendErr.Error(), "MEDIA_CAPTION_TOO_LONG") {
							req.Message = ""
							req.Entities = nil
							req.ReplyMarkup = nil
							_, _ = bm.api.MessagesSendMedia(ctx, req)
							return bm.sendTextWithMarkup(ctx, peer, caption, replyMarkup)
						}
						// Only delete from DB if the message was deleted from Telegram
						if strings.Contains(sendErr.Error(), "MESSAGE_ID_INVALID") {
							_ = bm.database.DeleteReelMedia(ctx, reelMsgID)
						}
					}
				} else {
					log.Printf("[Reel] Message %d in reel channel has no media or could not be loaded", reelMsgID)
				}
			}
		}
	}

	// Fallback to text message
	return bm.sendTextWithMarkup(ctx, peer, caption, replyMarkup)
}
