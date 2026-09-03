package bot

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"filetolink-go/internal/db"
	"filetolink-go/internal/markup"
	"filetolink-go/internal/pool"

	"github.com/gotd/td/tg"
)

func (bm *BotManager) handleCommand(
	ctx context.Context,
	cmd string,
	args []string,
	senderID int64,
	chatID int64,
	isPrivate bool,
	msg *tg.Message,
) error {
	peer := toInputPeer(chatID)

	switch cmd {
	case "/start":
		if isPrivate {
			_ = bm.database.SaveUserStart(ctx, senderID, "", "", "")
		}

		var rows [][]tg.KeyboardButtonClass
		rows = append(rows, []tg.KeyboardButtonClass{
			markup.NewURLButtonWithStyle(markup.ToSmallCaps("Updates Channel"), "https://t.me/Anime_Canon", markup.StyleGreen),
			markup.NewCallbackButtonWithStyle(markup.ToSmallCaps("Close"), "close", markup.StyleRed),
		})

		welcomeText := "✨ <b>Welcome to FileToLink Pro!</b> ✨\n\n" +
			"Send me any file, video, or document to get instant, permanent streaming and download links.\n\n" +
			"⚡ <b>Key Features:</b>\n" +
			"• High-speed parallel cloud streaming\n" +
			"• Permanent links with instant seek & subtitle tracks\n" +
			"• Works seamlessly in authorized groups & channels"

		_, err := bm.sendTextWithMarkup(ctx, peer, welcomeText, markup.NewInlineMarkup(rows))
		return err

	case "/ping":
		start := time.Now()
		msgID, err := bm.sendTextWithMarkup(ctx, peer, "🛰️ <b>Pinging...</b>", nil)
		if err == nil {
			elapsed := time.Since(start).Milliseconds()
			_ = bm.editMessage(ctx, peer, msgID, fmt.Sprintf("☁️ <b>PONG!</b> <code>%d ms</code>\n🤖 <b>Status:</b> <code>Active</code>", elapsed), nil)
		}
		return err

	case "/help":
		helpText := "📖 <b>Help Guide</b>\n\n" +
			"• <b>Private:</b> Send any media file directly to generate links.\n" +
			"• <b>Groups:</b> Reply to any file with <code>/link</code>.\n" +
			"• <b>Batch:</b> Reply with <code>/link 5</code> to process 5 consecutive files."
		return bm.sendText(ctx, peer, helpText)

	case "/about":
		aboutText := "🌟 <b>FileToLink Go Edition</b>\n\n" +
			"• High-throughput Go MTProto streaming service\n" +
			"• Zero-database stateless encrypted link architecture\n" +
			"• Built with ❤️ by Google Antigravity & Deepmind"
		return bm.sendText(ctx, peer, aboutText)

	case "/link":
		return bm.handleLinkCommand(ctx, msg, senderID, chatID, args)

	// -------------------------------------------------------------------------
	// Admin Commands (Restricted to Owner)
	// -------------------------------------------------------------------------

	case "/auth_gc", "/authorize_gc":
		if senderID != bm.cfg.OwnerID {
			return bm.sendText(ctx, peer, "❌ Unauthorized.")
		}
		targetID := chatID
		if len(args) > 0 {
			if id, err := strconv.ParseInt(args[0], 10, 64); err == nil {
				targetID = id
			}
		}
		_ = bm.database.AuthorizeGC(ctx, targetID, senderID)
		return bm.sendText(ctx, peer, fmt.Sprintf("✅ Group <code>%d</code> authorized.", targetID))

	case "/deauth_gc", "/deauthorize_gc":
		if senderID != bm.cfg.OwnerID {
			return bm.sendText(ctx, peer, "❌ Unauthorized.")
		}
		targetID := chatID
		if len(args) > 0 {
			if id, err := strconv.ParseInt(args[0], 10, 64); err == nil {
				targetID = id
			}
		}
		_ = bm.database.DeauthorizeGC(ctx, targetID)
		return bm.sendText(ctx, peer, fmt.Sprintf("✅ Group <code>%d</code> deauthorized.", targetID))

	case "/listauth_gc":
		if senderID != bm.cfg.OwnerID {
			return bm.sendText(ctx, peer, "❌ Unauthorized.")
		}
		gcs, err := bm.database.ListAuthorizedGCs(ctx)
		if err != nil || len(gcs) == 0 {
			return bm.sendText(ctx, peer, "ℹ️ No authorized group chats found.")
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("🔐 <b>Authorized Groups (%d):</b>\n\n", len(gcs)))
		for i, gc := range gcs {
			sb.WriteString(fmt.Sprintf("%d. <code>%d</code> (added %s)\n", i+1, gc.ChatID, gc.AuthorizedAt.Format("2006-01-02")))
		}
		return bm.sendText(ctx, peer, sb.String())

	case "/addpaid":
		if senderID != bm.cfg.OwnerID {
			return bm.sendText(ctx, peer, "❌ Unauthorized.")
		}
		if len(args) < 2 {
			return bm.sendText(ctx, peer, "⚠️ <b>Usage:</b> <code>/addpaid &lt;user_id&gt; &lt;duration (e.g. 30d, 1m, 3600s)&gt;</code>")
		}
		targetID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil || targetID == 0 {
			return bm.sendText(ctx, peer, "❌ Invalid user ID.")
		}
		durStr := strings.Join(args[1:], "")
		dur, err := ParseDurationString(durStr)
		if err != nil {
			return bm.sendText(ctx, peer, fmt.Sprintf("❌ Invalid duration: %v", err))
		}

		expiresAt := time.Now().Add(dur)
		_ = bm.database.AddPremium(ctx, targetID, expiresAt, senderID)

		targetPeer := toInputPeer(targetID)
		_ = bm.sendText(ctx, targetPeer, fmt.Sprintf("🎉 <b>Congratulations!</b> You have been upgraded to Premium for <code>%s</code>!\nExpires at: %s",
			durStr, expiresAt.Format("2006-01-02 15:04:05 MST")))

		return bm.sendText(ctx, peer, fmt.Sprintf("✅ Premium activated for User <code>%d</code> until %s",
			targetID, expiresAt.Format("2006-01-02 15:04:05 MST")))

	case "/removepaid":
		if senderID != bm.cfg.OwnerID {
			return bm.sendText(ctx, peer, "❌ Unauthorized.")
		}
		if len(args) < 1 {
			return bm.sendText(ctx, peer, "⚠️ <b>Usage:</b> <code>/removepaid &lt;user_id&gt;</code>")
		}
		targetID, _ := strconv.ParseInt(args[0], 10, 64)
		_ = bm.database.RemovePremium(ctx, targetID)
		return bm.sendText(ctx, peer, fmt.Sprintf("✅ Premium access removed for user <code>%d</code>.", targetID))

	case "/listpaid":
		if senderID != bm.cfg.OwnerID {
			return bm.sendText(ctx, peer, "❌ Unauthorized.")
		}
		users, err := bm.database.ListPremiumUsers(ctx)
		if err != nil || len(users) == 0 {
			return bm.sendText(ctx, peer, "ℹ️ No active paid users found.")
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("💎 <b>Active Premium Users (%d):</b>\n\n", len(users)))
		for i, u := range users {
			timeLeft := FormatTimeLeft(u.ExpiresAt)
			sb.WriteString(fmt.Sprintf("%d. 👤 <code>%d</code> - ⏳ %s\n", i+1, u.UserID, timeLeft))
		}
		return bm.sendText(ctx, peer, sb.String())

	case "/pmmode":
		if senderID != bm.cfg.OwnerID {
			return bm.sendText(ctx, peer, "❌ Unauthorized.")
		}
		if len(args) == 0 {
			cur := bm.database.GetPMMode(ctx, bm.cfg.PMModeDefault)
			return bm.sendText(ctx, peer, fmt.Sprintf("Current PM Mode: <code>%v</code>\nUsage: <code>/pmmode on|off</code>", cur))
		}
		mode := strings.ToLower(args[0]) == "on" || args[0] == "true" || args[0] == "1"
		_ = bm.database.SetPMMode(ctx, mode)
		return bm.sendText(ctx, peer, fmt.Sprintf("✅ PM Mode updated to: <b>%v</b>", mode))

	case "/stats":
		if senderID != bm.cfg.OwnerID {
			return bm.sendText(ctx, peer, "❌ Unauthorized.")
		}
		traffic, _ := bm.database.GetTrafficStats(ctx)
		userCount := bm.database.TotalUsers(ctx)
		sessions := bm.pool.ActiveSessionCount()

		var tStats db.TrafficStats
		if traffic != nil {
			tStats = *traffic
		}

		statsText := fmt.Sprintf("📊 <b>FileToLink System Statistics</b>\n\n"+
			"👥 <b>Total Users:</b> <code>%d</code>\n"+
			"🤖 <b>Active Bot Sessions:</b> <code>%d</code>\n\n"+
			"📈 <b>Bandwidth Transferred:</b>\n"+
			"• Today: <code>%s</code>\n"+
			"• This Week: <code>%s</code>\n"+
			"• This Month: <code>%s</code>\n"+
			"• This Year: <code>%s</code>\n"+
			"• Overall: <code>%s</code>",
			userCount, sessions,
			humanBytes(tStats.Today),
			humanBytes(tStats.ThisWeek),
			humanBytes(tStats.ThisMonth),
			humanBytes(tStats.ThisYear),
			humanBytes(tStats.Overall),
		)
		return bm.sendText(ctx, peer, statsText)
	}

	return nil
}

func (bm *BotManager) handleLinkCommand(
	ctx context.Context,
	msg *tg.Message,
	senderID int64,
	chatID int64,
	args []string,
) error {
	peer := toInputPeer(chatID)

	// Check if group is authorized
	if !bm.database.IsGCAuthorized(ctx, chatID) && senderID != bm.cfg.OwnerID {
		return bm.sendText(ctx, peer, "⚠️ <b>This group is not authorized to use /link.</b>")
	}

	// Check start in DM
	if !bm.database.HasUserStarted(ctx, senderID) && senderID != bm.cfg.OwnerID {
		bm.sendStartInDMPrompt(ctx, peer, msg.ID)
		return nil
	}

	replyHeader := msg.ReplyTo
	if replyHeader == nil {
		return bm.sendText(ctx, peer, "⚠️ <b>Please reply to a media file with /link.</b>")
	}

	replyH, ok := replyHeader.(*tg.MessageReplyHeader)
	if !ok || replyH.ReplyToMsgID == 0 {
		return bm.sendText(ctx, peer, "⚠️ <b>Please reply to a media file with /link.</b>")
	}

	res, err := bm.api.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
		Channel: &tg.InputChannel{ChannelID: pool.RawChannelID(chatID)},
		ID:      []tg.InputMessageClass{&tg.InputMessageID{ID: replyH.ReplyToMsgID}},
	})
	if err != nil {
		return bm.sendText(ctx, peer, "❌ Could not fetch replied message.")
	}

	var targetMsg *tg.Message
	if chMsgs, ok := res.(*tg.MessagesChannelMessages); ok && len(chMsgs.Messages) > 0 {
		targetMsg, _ = chMsgs.Messages[0].(*tg.Message)
	}

	if targetMsg == nil || targetMsg.Media == nil {
		return bm.sendText(ctx, peer, "⚠️ The replied-to message contains no media file.")
	}

	return bm.handleMedia(ctx, targetMsg, senderID, chatID, true)
}
