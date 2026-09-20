package bot

import (
	"context"
	"fmt"
	"log"

	"filetolink-go/internal/crypto"
	"filetolink-go/internal/markup"

	"github.com/gotd/td/tg"
)

func (bm *BotManager) handleMedia(ctx context.Context, msg *tg.Message, senderID, chatID int64, isPrivate bool) error {
	peer := toInputPeer(chatID)

	// 1. Permission check
	if isPrivate {
		if !bm.isAllowedInPM(ctx, senderID) {
			var rows [][]tg.KeyboardButtonClass
			rows = append(rows, []tg.KeyboardButtonClass{
				markup.NewURLButtonWithStyle(markup.ToSmallCaps("Join Community"), "https://t.me/Canon_Bots", markup.StyleGreen),
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

	dyn := bm.GetSettings()
	isPaid := bm.database.IsPremiumUser(ctx, senderID) || senderID == bm.cfg.OwnerID

	// 3.1 Token Verification Check (Shorten Enable / Token TTL)
	if dyn.TokenEnabled && !isPaid {
		if !bm.database.IsUserTokenVerified(ctx, senderID) {
			token, err := bm.database.CreateVerificationToken(ctx, senderID, dyn.TokenTTLHours)
			if err == nil {
				botUsername := "bot"
				if bm.botUser != nil && bm.botUser.Username != "" {
					botUsername = bm.botUser.Username
				}
				verifyURL := fmt.Sprintf("https://t.me/%s?start=verify_%s", botUsername, token)
				shortURL := bm.shortener.Shorten(ctx, verifyURL)

				verifyText := fmt.Sprintf("🔐 <b>Token Verification Required</b>\n\n"+
					"To generate streaming & download links for the next <b>%d hours</b>, please verify your access:\n\n"+
					"👉 <i>Click the button below and pass the link to unlock the bot.</i>", dyn.TokenTTLHours)

				var rows [][]tg.KeyboardButtonClass
				rows = append(rows, []tg.KeyboardButtonClass{
					markup.NewURLButtonWithStyle("👉 Verify Access Token", shortURL, markup.StyleGreen),
					markup.NewCallbackButtonWithStyle(markup.ToSmallCaps("Close"), "close", markup.StyleRed),
				})

				_, _ = bm.sendTextWithMarkup(ctx, peer, verifyText, markup.NewInlineMarkup(rows))
				return nil
			}
		}
	}

	// 4. Send initial status
	statusMsgID, err := bm.sendTextWithMarkup(ctx, peer, "⏳ <b>Processing your file...</b>", nil)
	if err != nil {
		return err
	}

	// 5. Forward media to BIN_CHANNEL with rate limit and FLOOD_WAIT protection
	fwdMsgID, fwdMsg, err := bm.ForwardToBinWithFloodWait(ctx, peer, msg.ID)
	if err != nil {
		log.Printf("[Bot] Failed to forward media to BIN_CHANNEL: %v", err)
		_ = bm.editMessage(ctx, peer, statusMsgID, "❌ <b>Failed to store media in storage channel:</b> "+err.Error(), nil)
		return err
	}
	binPeer := toInputPeer(bm.cfg.BinChannel)

	// 6. Extract file metadata
	var fileName string
	var fileSize int64
	if fwdMsg != nil {
		fileName, fileSize, _ = extractMediaInfo(fwdMsg)
	}
	if fileName == "" {
		fileName, fileSize, _ = extractMediaInfo(msg)
	}

	// 7. Generate compact stateless encrypted token (24 chars)
	token := crypto.EncryptCompactMessageID(int64(fwdMsgID), bm.cfg.EncryptionKey)

	// 8. Construct clean URLs
	baseURL := bm.cfg.BuildEffectiveBaseURL()
	streamURL := fmt.Sprintf("%s/watch/%s", baseURL, token)
	downloadURL := fmt.Sprintf("%s/dl/%s", baseURL, token)

	// Apply URL shortener for non-premium users if enabled
	if !isPaid && dyn.ShortenMediaLinks && bm.shortener != nil && bm.shortener.Enabled() {
		downloadURL = bm.shortener.Shorten(ctx, downloadURL)
		streamURL = bm.shortener.Shorten(ctx, streamURL)
	}

	streamable := isStreamable(fileName)

	// 8.1 Reply to stored media in BIN_CHANNEL with source user and links (FileToLink behavior)
	sourceName := fmt.Sprintf("User %d", senderID)
	var binLogText string
	if streamable {
		binLogText = fmt.Sprintf("<blockquote>👤 <b>Source:</b> <a href=\"tg://user?id=%d\">%s</a>\n"+
			"🆔 <b>ID:</b> <code>%d</code></blockquote>\n\n"+
			"<blockquote>🚀 <b>Download:</b> %s\n\n"+
			"🖥️ <b>Stream:</b> %s</blockquote>",
			senderID, sourceName, senderID, downloadURL, streamURL)
	} else {
		binLogText = fmt.Sprintf("<blockquote>👤 <b>Source:</b> <a href=\"tg://user?id=%d\">%s</a>\n"+
			"🆔 <b>ID:</b> <code>%d</code></blockquote>\n\n"+
			"<blockquote>🚀 <b>Download:</b> %s</blockquote>",
			senderID, sourceName, senderID, downloadURL)
	}

	plainBinText, binEntities := parseHTML(binLogText)
	_, _ = bm.api.MessagesSendMessage(ctx, &tg.MessagesSendMessageRequest{
		Peer:      binPeer,
		ReplyTo:   &tg.InputReplyToMessage{ReplyToMsgID: fwdMsgID},
		Message:   plainBinText,
		Entities:  binEntities,
		NoWebpage: true,
		RandomID:  getRandomID(),
	})

	// 9. Format response text & colorful buttons (professional quotes, no monospace, FDM/1DM pro tip)
	var text string
	var rows [][]tg.KeyboardButtonClass

	if streamable {
		text = fmt.Sprintf("✨ <b>Your Links are Ready!</b>\n\n"+
			"<blockquote>📁 <b>File Name:</b> %s\n"+
			"📦 <b>File Size:</b> %s</blockquote>\n\n"+
			"<blockquote>🚀 <b>Download Link:</b>\n%s\n\n"+
			"🖥️ <b>Watch Link:</b>\n%s</blockquote>\n\n"+
			"<blockquote>💡 <b>Pro Tip:</b> For maximum download speed, use <b>FDM (Free Download Manager)</b> on PC and <b>1DM+</b> on Mobile.</blockquote>",
			htmlEscape(fileName), humanBytes(fileSize), downloadURL, streamURL)

		rows = append(rows, []tg.KeyboardButtonClass{
			markup.NewURLButtonWithStyle(markup.ToSmallCaps("Stream"), streamURL, markup.StyleGreen),
			markup.NewURLButtonWithStyle(markup.ToSmallCaps("Download"), downloadURL, markup.StyleBlue),
		})
	} else {
		text = fmt.Sprintf("✨ <b>Your Link is Ready!</b>\n\n"+
			"<blockquote>📁 <b>File Name:</b> %s\n"+
			"📦 <b>File Size:</b> %s</blockquote>\n\n"+
			"<blockquote>🚀 <b>Download Link:</b>\n%s</blockquote>\n\n"+
			"<blockquote>💡 <b>Pro Tip:</b> For maximum download speed, use <b>FDM (Free Download Manager)</b> on PC and <b>1DM+</b> on Mobile.</blockquote>",
			htmlEscape(fileName), humanBytes(fileSize), downloadURL)

		rows = append(rows, []tg.KeyboardButtonClass{
			markup.NewURLButtonWithStyle(markup.ToSmallCaps("Download"), downloadURL, markup.StyleBlue),
		})
	}

	// 10. Reply with Reel if configured, otherwise edit the status message
	if bm.cfg.ReelChannelID != 0 {
		_ = bm.deleteMessages(ctx, peer, []int{statusMsgID})
		_, err = bm.replyWithReel(ctx, peer, msg.ID, text, markup.NewInlineMarkup(rows))
		return err
	}

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
	return bm.GetSettings().PMMode
}
