package bot

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"filetolink-go/internal/markup"

	"github.com/gotd/td/tg"
)

func (bm *BotManager) handleDC(ctx context.Context, msg *tg.Message, senderID, chatID int64, args []string) error {
	peer := toInputPeer(chatID)

	// Check if user is banned
	if bm.database.IsBanned(ctx, senderID) {
		return nil
	}

	// Check force-sub
	if !bm.checkFSub(ctx, senderID, peer) {
		return nil
	}

	// 1. If explicit user query was passed (e.g. /dc @username or /dc 12345678)
	if len(args) > 0 {
		target := strings.TrimSpace(args[0])
		if strings.HasPrefix(target, "@") {
			username := strings.TrimPrefix(target, "@")
			res, err := bm.api.ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{
				Username: username,
			})
			if err == nil && res != nil && len(res.Users) > 0 {
				if u, ok := res.Users[0].(*tg.User); ok {
					return bm.sendUserDC(ctx, peer, u)
				}
			}
			return bm.sendText(ctx, peer, fmt.Sprintf("❌ <b>Could not find user details for:</b> <code>@%s</code>", username))
		}

		if uid, err := strconv.ParseInt(target, 10, 64); err == nil {
			return bm.sendUserDCByID(ctx, peer, uid)
		}
	}

	// 2. If replying to a message
	if msg.ReplyTo != nil {
		if header, ok := msg.ReplyTo.(*tg.MessageReplyHeader); ok && header.ReplyToMsgID != 0 {
			res, err := bm.api.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
				Channel: toInputChannel(chatID),
				ID:      []tg.InputMessageClass{&tg.InputMessageID{ID: header.ReplyToMsgID}},
			})
			if err != nil {
				resNormal, errNormal := bm.api.MessagesGetMessages(ctx, []tg.InputMessageClass{&tg.InputMessageID{ID: header.ReplyToMsgID}})
				if errNormal == nil {
					res = resNormal
				}
			}

			if res != nil {
				msgs := extractMessagesFromClass(res)
				if len(msgs) > 0 {
					repliedMsg := msgs[0]

					// If replied message has media -> Show File DC
					if repliedMsg.Media != nil {
						return bm.sendFileDC(ctx, peer, repliedMsg)
					}

					// If replied message is from a user -> Show that user's DC
					if repliedMsg.FromID != nil {
						if pu, ok := repliedMsg.FromID.(*tg.PeerUser); ok {
							return bm.sendUserDCByID(ctx, peer, pu.UserID)
						}
					}
				}
			}
		}
		return bm.sendText(ctx, peer, "🤔 <b>Invalid Usage:</b> Please reply to a user's message or a media file to get DC info.")
	}

	// 3. Otherwise show the sender's own DC info
	return bm.sendUserDCByID(ctx, peer, senderID)
}

func (bm *BotManager) sendUserDCByID(ctx context.Context, peer tg.InputPeerClass, userID int64) error {
	var user *tg.User

	// Try fetching user from Telegram API
	res, err := bm.api.UsersGetUsers(ctx, []tg.InputUserClass{
		&tg.InputUser{UserID: userID, AccessHash: 0},
	})
	if err == nil && res != nil {
		for _, uClass := range res {
			if u, ok := uClass.(*tg.User); ok && u.ID == userID {
				user = u
				break
			}
		}
	}

	if user != nil {
		return bm.sendUserDC(ctx, peer, user)
	}

	// Fallback when detailed user object cannot be resolved without access_hash
	text := fmt.Sprintf("📍 <b>Information</b>\n"+
		"<blockquote>👤 <b>User:</b> <a href=\"tg://user?id=%d\">User</a>\n"+
		"🆔 <b>User ID:</b> <code>%d</code>\n"+
		"🌍 <b>DC ID:</b> <code>Unknown</code></blockquote>",
		userID, userID)

	profileURL := fmt.Sprintf("tg://user?id=%d", userID)
	var rows [][]tg.KeyboardButtonClass
	rows = append(rows, []tg.KeyboardButtonClass{
		markup.NewURLButtonWithStyle(markup.ToSmallCaps("View Profile"), profileURL, markup.StyleBlue),
	})
	rows = append(rows, []tg.KeyboardButtonClass{
		markup.NewCallbackButtonWithStyle(markup.ToSmallCaps("Close"), "close", markup.StyleRed),
	})

	_, err = bm.sendTextWithMarkup(ctx, peer, text, markup.NewInlineMarkup(rows))
	return err
}

func (bm *BotManager) sendUserDC(ctx context.Context, peer tg.InputPeerClass, user *tg.User) error {
	var dcStr = "Unknown"
	if photo, ok := user.Photo.(*tg.UserProfilePhoto); ok && photo.DCID != 0 {
		dcStr = fmt.Sprintf("%d", photo.DCID)
	}

	userName := user.FirstName
	if userName == "" {
		userName = "User"
	}

	var profileURL string
	if user.Username != "" {
		profileURL = fmt.Sprintf("https://t.me/%s", user.Username)
	} else {
		profileURL = fmt.Sprintf("tg://user?id=%d", user.ID)
	}

	text := fmt.Sprintf("📍 <b>Information</b>\n"+
		"<blockquote>👤 <b>User:</b> <a href=\"%s\">%s</a>\n"+
		"🆔 <b>User ID:</b> <code>%d</code>\n"+
		"🌍 <b>DC ID:</b> <code>%s</code></blockquote>",
		profileURL, htmlEscape(userName), user.ID, dcStr)

	var rows [][]tg.KeyboardButtonClass
	rows = append(rows, []tg.KeyboardButtonClass{
		markup.NewURLButtonWithStyle(markup.ToSmallCaps("View Profile"), profileURL, markup.StyleBlue),
	})
	rows = append(rows, []tg.KeyboardButtonClass{
		markup.NewCallbackButtonWithStyle(markup.ToSmallCaps("Close"), "close", markup.StyleRed),
	})

	_, err := bm.sendTextWithMarkup(ctx, peer, text, markup.NewInlineMarkup(rows))
	return err
}

func (bm *BotManager) sendFileDC(ctx context.Context, peer tg.InputPeerClass, fileMsg *tg.Message) error {
	fileName, fileSize, fileType, dcID := extractFileDCInfo(fileMsg)
	var dcStr = "Unknown"
	if dcID > 0 {
		dcStr = fmt.Sprintf("%d", dcID)
	}

	text := fmt.Sprintf("🗂️ <b>File Information</b>\n"+
		"<blockquote><code>%s</code></blockquote>\n"+
		"💾 <b>File Size:</b> <code>%s</code>\n"+
		"📁 <b>File Type:</b> <code>%s</code>\n"+
		"🌍 <b>DC ID:</b> <code>%s</code>",
		htmlEscape(fileName), humanBytes(fileSize), fileType, dcStr)

	var rows [][]tg.KeyboardButtonClass
	rows = append(rows, []tg.KeyboardButtonClass{
		markup.NewCallbackButtonWithStyle(markup.ToSmallCaps("Close"), "close", markup.StyleRed),
	})

	_, err := bm.sendTextWithMarkup(ctx, peer, text, markup.NewInlineMarkup(rows))
	return err
}

func extractFileDCInfo(msg *tg.Message) (fileName string, fileSize int64, fileType string, dcID int) {
	if msg == nil || msg.Media == nil {
		return "Unknown", 0, "Unknown", 0
	}

	switch m := msg.Media.(type) {
	case *tg.MessageMediaDocument:
		if doc, ok := m.Document.(*tg.Document); ok {
			fileSize = doc.Size
			fileName = fmt.Sprintf("file_%d.bin", doc.ID)
			fileType = "Document"
			dcID = doc.DCID
			for _, attr := range doc.Attributes {
				switch a := attr.(type) {
				case *tg.DocumentAttributeFilename:
					fileName = a.FileName
				case *tg.DocumentAttributeVideo:
					fileType = "Video"
				case *tg.DocumentAttributeAudio:
					if a.Voice {
						fileType = "Voice"
					} else {
						fileType = "Audio"
					}
				case *tg.DocumentAttributeSticker:
					fileType = "Sticker"
				case *tg.DocumentAttributeAnimated:
					fileType = "Animation"
				}
			}
			return
		}
	case *tg.MessageMediaPhoto:
		if photo, ok := m.Photo.(*tg.Photo); ok {
			fileName = fmt.Sprintf("photo_%d.jpg", photo.ID)
			fileType = "Photo"
			dcID = photo.DCID
			fileSize = 1024 * 1024
			return
		}
	}

	return "Unknown", 0, "Unknown", 0
}
