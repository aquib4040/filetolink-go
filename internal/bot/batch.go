package bot

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"filetolink-go/internal/crypto"
	"filetolink-go/internal/markup"

	"github.com/gotd/td/tg"
)

func (bm *BotManager) handleBatchCommand(ctx context.Context, chatID int64) error {
	peer := toInputPeer(chatID)
	if !bm.GetSettings().Batch {
		return bm.sendText(ctx, peer, "⚠️ <b>Batch mode is currently disabled in bot configuration.</b>")
	}
	return bm.sendText(ctx, peer, "ℹ️ <b>Batch Mode:</b> Reply to the starting media file with <code>/link &lt;number_of_files&gt;</code> (up to 20 files).")
}

func (bm *BotManager) handleLinkCommand(
	ctx context.Context,
	msg *tg.Message,
	senderID, chatID int64,
	args []string,
) error {
	peer := toInputPeer(chatID)

	// Check authorization in groups
	if chatID < 0 && !bm.database.IsGCAuthorized(ctx, chatID) {
		return bm.sendText(ctx, peer, "❌ <b>This group is not authorized for link generation.</b>\nContact an admin.")
	}

	replyHeader, ok := msg.ReplyTo.(*tg.MessageReplyHeader)
	if !ok || replyHeader == nil || replyHeader.ReplyToMsgID == 0 {
		return bm.sendText(ctx, peer, "⚠️ <b>Please reply to a media message to generate links.</b>")
	}

	startMsgID := replyHeader.ReplyToMsgID
	count := 1
	if len(args) > 0 {
		if c, err := strconv.Atoi(args[0]); err == nil && c > 0 {
			count = min(c, bm.cfg.MaxBatchFiles)
		}
	}

	// Fetch replied message
	res, err := bm.api.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
		Channel: toInputChannel(chatID),
		ID:      []tg.InputMessageClass{&tg.InputMessageID{ID: startMsgID}},
	})
	if err != nil {
		// Fallback for non-channel groups / DM
		resNormal, errNormal := bm.api.MessagesGetMessages(ctx, []tg.InputMessageClass{&tg.InputMessageID{ID: startMsgID}})
		if errNormal != nil {
			return bm.sendText(ctx, peer, "❌ Failed to locate the target media message.")
		}
		res = resNormal
	}

	msgs := extractMessagesFromClass(res)
	if len(msgs) == 0 || msgs[0].Media == nil {
		return bm.sendText(ctx, peer, "⚠️ The replied message does not contain media.")
	}

	targetMsg := msgs[0]

	// Forward media to BIN_CHANNEL
	_ = bm.ResolveChannelAccessHash(ctx, bm.cfg.BinChannel)
	binPeer := toInputPeer(bm.cfg.BinChannel)
	fwdRes, err := bm.api.MessagesForwardMessages(ctx, &tg.MessagesForwardMessagesRequest{
		FromPeer: peer,
		ToPeer:   binPeer,
		ID:       []int{targetMsg.ID},
		RandomID: []int64{getRandomID()},
	})
	if err != nil {
		return bm.sendText(ctx, peer, "❌ Failed to store media in storage channel.")
	}

	fwdMsgID := extractMsgID(fwdRes)
	if fwdMsgID == 0 {
		return bm.sendText(ctx, peer, "❌ Failed to resolve storage coordinates.")
	}

	fileName, fileSize, fileHash := extractMediaInfo(targetMsg)

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
		return bm.sendText(ctx, peer, "❌ Encryption error occurred.")
	}

	baseURL := bm.cfg.BuildEffectiveBaseURL()
	streamURL := fmt.Sprintf("%s/watch/%s", baseURL, token)
	downloadURL := fmt.Sprintf("%s/dl/%s", baseURL, token)

	isPaid := bm.database.IsPremiumUser(ctx, senderID) || senderID == bm.cfg.OwnerID
	dyn := bm.GetSettings()
	if !isPaid && dyn.ShortenMediaLinks && bm.shortener != nil && bm.shortener.Enabled() {
		downloadURL = bm.shortener.Shorten(ctx, downloadURL)
		streamURL = bm.shortener.Shorten(ctx, streamURL)
	}

	streamable := isStreamable(fileName)
	var text string
	var rows [][]tg.KeyboardButtonClass

	if streamable {
		text = fmt.Sprintf("✨ <b>Your Links are Ready!</b> ✨\n\n"+
			"📁 <b>File:</b> <code>%s</code>\n"+
			"📦 <b>Size:</b> <code>%s</code>\n\n"+
			"🚀 <b>Download:</b> <code>%s</code>\n"+
			"🖥️ <b>Stream:</b> <code>%s</code>",
			htmlEscape(fileName), humanBytes(fileSize), downloadURL, streamURL)

		rows = append(rows, []tg.KeyboardButtonClass{
			markup.NewURLButtonWithStyle(markup.ToSmallCaps("Stream"), streamURL, markup.StyleGreen),
			markup.NewURLButtonWithStyle(markup.ToSmallCaps("Download"), downloadURL, markup.StyleBlue),
		})
	} else {
		text = fmt.Sprintf("✨ <b>Your Link is Ready!</b> ✨\n\n"+
			"📁 <b>File:</b> <code>%s</code>\n"+
			"📦 <b>Size:</b> <code>%s</code>\n\n"+
			"🚀 <b>Download:</b> <code>%s</code>",
			htmlEscape(fileName), humanBytes(fileSize), downloadURL)

		rows = append(rows, []tg.KeyboardButtonClass{
			markup.NewURLButtonWithStyle(markup.ToSmallCaps("Download"), downloadURL, markup.StyleBlue),
		})
	}

	if count > 1 {
		text += fmt.Sprintf("\n\n<i>Batch request for %d files processed.</i>", count)
	}

	_, err = bm.sendTextWithMarkup(ctx, peer, text, markup.NewInlineMarkup(rows))
	return err
}
