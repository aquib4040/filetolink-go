package bot

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
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

		// Token verification callback: /start verify_<token>
		if len(args) > 0 && strings.HasPrefix(args[0], "verify_") {
			tok := strings.TrimPrefix(args[0], "verify_")
			dyn := bm.GetSettings()
			activated, _, err := bm.database.ActivateVerificationToken(ctx, tok, dyn.TokenTTLHours)
			if activated && err == nil {
				return bm.sendText(ctx, peer, fmt.Sprintf("✅ <b>Successfully Verified!</b>\n\nYou now have full access to generate links for the next <b>%d hours</b>. Send any media file to begin!", dyn.TokenTTLHours))
			}
			return bm.sendText(ctx, peer, "❌ <b>Invalid or expired verification token.</b> Please request a new verification link.")
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

	case "/speedtest":
		if senderID != bm.cfg.OwnerID {
			return bm.sendText(ctx, peer, "❌ Unauthorized.")
		}
		msgID, err := bm.sendTextWithMarkup(ctx, peer, "🚀 <b>Running Speed Test...</b>\n<i>Testing network latency and CDN throughput...</i>", nil)
		if err != nil {
			return err
		}
		go func() {
			ping, speedMbps := runNetworkSpeedTest()
			res := fmt.Sprintf("⚡ <b>SPEEDTEST RESULTS:</b>\n\n"+
				"📶 <b>Ping Latency:</b> <code>%d ms</code>\n"+
				"📥 <b>Download Speed:</b> <code>%.2f Mbps</code>\n"+
				"🌐 <b>Host:</b> <code>Cloud Container / Heroku</code>\n"+
				"🛰️ <b>Status:</b> <code>Optimal</code>", ping, speedMbps)
			_ = bm.editMessage(ctx, peer, msgID, res, nil)
		}()
		return nil

	case "/restart":
		if senderID != bm.cfg.OwnerID {
			return bm.sendText(ctx, peer, "❌ Unauthorized.")
		}
		msgID, _ := bm.sendTextWithMarkup(ctx, peer, "♻️ <b>Updating and Restarting Bot...</b>\n\n> ⏳ <i>Please wait a moment.</i>", nil)
		_ = bm.database.SaveRestartMessage(ctx, int64(msgID), chatID)
		go func() {
			time.Sleep(1 * time.Second)
			os.Exit(0)
		}()
		return nil

	case "/ban":
		if senderID != bm.cfg.OwnerID {
			return bm.sendText(ctx, peer, "❌ Unauthorized.")
		}
		if len(args) < 1 {
			return bm.sendText(ctx, peer, "⚠️ <b>Usage:</b> <code>/ban &lt;user_id&gt; [reason]</code>")
		}
		targetID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil || targetID == 0 {
			return bm.sendText(ctx, peer, "❌ Invalid user ID.")
		}
		reason := "Violation of terms"
		if len(args) > 1 {
			reason = strings.Join(args[1:], " ")
		}
		_ = bm.database.BanUser(ctx, targetID, reason)
		return bm.sendText(ctx, peer, fmt.Sprintf("⛔ User <code>%d</code> has been banned.\nReason: <i>%s</i>", targetID, reason))

	case "/unban":
		if senderID != bm.cfg.OwnerID {
			return bm.sendText(ctx, peer, "❌ Unauthorized.")
		}
		if len(args) < 1 {
			return bm.sendText(ctx, peer, "⚠️ <b>Usage:</b> <code>/unban &lt;user_id&gt;</code>")
		}
		targetID, _ := strconv.ParseInt(args[0], 10, 64)
		_ = bm.database.UnbanUser(ctx, targetID)
		return bm.sendText(ctx, peer, fmt.Sprintf("✅ User <code>%d</code> has been unbanned.", targetID))

	case "/listbanned", "/banned":
		if senderID != bm.cfg.OwnerID {
			return bm.sendText(ctx, peer, "❌ Unauthorized.")
		}
		banned, err := bm.database.ListBannedUsers(ctx)
		if err != nil || len(banned) == 0 {
			return bm.sendText(ctx, peer, "ℹ️ No banned users found.")
		}
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("⛔ <b>Banned Users (%d):</b>\n\n", len(banned)))
		for i, b := range banned {
			sb.WriteString(fmt.Sprintf("%d. <code>%d</code> - %s (%s)\n", i+1, b.UserID, b.Reason, b.BannedAt.Format("2006-01-02")))
		}
		return bm.sendText(ctx, peer, sb.String())

	case "/fsub":
		if senderID != bm.cfg.OwnerID {
			return bm.sendText(ctx, peer, "❌ Unauthorized.")
		}
		return bm.sendFSubSettingsPanel(ctx, peer)

	case "/settings":
		if senderID != bm.cfg.OwnerID {
			return bm.sendText(ctx, peer, "❌ Unauthorized.")
		}
		return bm.sendMainSettingsPanel(ctx, peer, 0)

	case "/set_shortener":
		if senderID != bm.cfg.OwnerID {
			return bm.sendText(ctx, peer, "❌ Unauthorized.")
		}
		if len(args) < 2 {
			return bm.sendText(ctx, peer, "⚠️ <b>Usage:</b> <code>/set_shortener &lt;site&gt; &lt;api_key&gt;</code>")
		}
		_ = bm.UpdateSetting(ctx, "shortener_site", args[0])
		_ = bm.UpdateSetting(ctx, "shortener_api_key", args[1])
		return bm.sendText(ctx, peer, fmt.Sprintf("✅ URL Shortener updated:\nSite: <code>%s</code>\nKey: <code>%s...</code>", args[0], args[1][:min(len(args[1]), 6)]))

	case "/set_ttl":
		if senderID != bm.cfg.OwnerID {
			return bm.sendText(ctx, peer, "❌ Unauthorized.")
		}
		if len(args) < 1 {
			return bm.sendText(ctx, peer, "⚠️ <b>Usage:</b> <code>/set_ttl &lt;hours&gt;</code>")
		}
		hrs, err := strconv.Atoi(args[0])
		if err != nil || hrs <= 0 {
			return bm.sendText(ctx, peer, "❌ Invalid hours value.")
		}
		_ = bm.UpdateSetting(ctx, "token_ttl_hours", hrs)
		return bm.sendText(ctx, peer, fmt.Sprintf("✅ Token validity TTL updated to: <b>%d Hours</b>", hrs))

	case "/set_fsub":
		if senderID != bm.cfg.OwnerID {
			return bm.sendText(ctx, peer, "❌ Unauthorized.")
		}
		if len(args) < 2 {
			return bm.sendText(ctx, peer, "⚠️ <b>Usage:</b> <code>/set_fsub &lt;channel_id&gt; &lt;invite_link&gt;</code>")
		}
		chID, err := strconv.ParseInt(args[0], 10, 64)
		if err != nil || chID == 0 {
			return bm.sendText(ctx, peer, "❌ Invalid channel ID.")
		}
		inv := args[1]
		_ = bm.database.AddFSubChannel(ctx, db.FSubChannel{
			ChannelID: chID,
			Title:     fmt.Sprintf("Channel %d", chID),
			InviteURL: inv,
		})
		return bm.sendText(ctx, peer, fmt.Sprintf("✅ Force-Sub channel <code>%d</code> added.", chID))

	case "/rm_fsub":
		if senderID != bm.cfg.OwnerID {
			return bm.sendText(ctx, peer, "❌ Unauthorized.")
		}
		if len(args) < 1 {
			return bm.sendText(ctx, peer, "⚠️ <b>Usage:</b> <code>/rm_fsub &lt;channel_id&gt;</code>")
		}
		chID, _ := strconv.ParseInt(args[0], 10, 64)
		_ = bm.database.RemoveFSubChannel(ctx, chID)
		return bm.sendText(ctx, peer, fmt.Sprintf("✅ Force-Sub channel <code>%d</code> removed.", chID))

	case "/batch":
		if !bm.GetSettings().Batch {
			return bm.sendText(ctx, peer, "⚠️ <b>Batch mode is currently disabled in bot configuration.</b>")
		}
		return bm.sendText(ctx, peer, "ℹ️ <b>Batch Mode:</b> Reply to the starting media file with <code>/link &lt;number_of_files&gt;</code> (up to 20 files).")
	}

	return nil
}

func (bm *BotManager) sendMainSettingsPanel(ctx context.Context, peer tg.InputPeerClass, msgID int) error {
	dyn := bm.GetSettings()

	smlStatus := "❌ Disabled"
	if dyn.ShortenMediaLinks {
		smlStatus = "✅ Enabled"
	}

	tokStatus := "❌ Disabled"
	if dyn.TokenEnabled {
		tokStatus = "✅ Enabled"
	}

	pmStatus := "❌ Off (Restricted)"
	if dyn.PMMode {
		pmStatus = "✅ On (Public)"
	}

	batchStatus := "❌ Disabled"
	if dyn.Batch {
		batchStatus = "✅ Enabled"
	}

	site := dyn.ShortenerSite
	if site == "" {
		site = "Not Configured"
	}

	text := fmt.Sprintf("⚙️ <b>Bot Control Panel & Settings</b>\n\n"+
		"• <b>Shorten Media Links:</b> <code>%s</code>\n"+
		"• <b>Token Verification:</b> <code>%s</code>\n"+
		"• <b>Token Validity (TTL):</b> <code>%d Hours</code>\n"+
		"• <b>PM Mode (Direct Messages):</b> <code>%s</code>\n"+
		"• <b>Batch Processing:</b> <code>%s</code>\n"+
		"• <b>URL Shortener Site:</b> <code>%s</code>\n\n"+
		"<i>Click buttons below to toggle options immediately. Saved to MongoDB!</i>",
		smlStatus, tokStatus, dyn.TokenTTLHours, pmStatus, batchStatus, site)

	var rows [][]tg.KeyboardButtonClass

	btnSML := "🔗 Shorten Links: OFF"
	if dyn.ShortenMediaLinks {
		btnSML = "🔗 Shorten Links: ON"
	}
	btnTok := "🔑 Token Auth: OFF"
	if dyn.TokenEnabled {
		btnTok = "🔑 Token Auth: ON"
	}
	rows = append(rows, []tg.KeyboardButtonClass{
		markup.NewCallbackButtonWithStyle(btnSML, "set_toggle_sml", markup.StyleBlue),
		markup.NewCallbackButtonWithStyle(btnTok, "set_toggle_token", markup.StyleBlue),
	})

	btnPM := "💬 PM Mode: OFF"
	if dyn.PMMode {
		btnPM = "💬 PM Mode: ON"
	}
	btnBatch := "📦 Batch: OFF"
	if dyn.Batch {
		btnBatch = "📦 Batch: ON"
	}
	rows = append(rows, []tg.KeyboardButtonClass{
		markup.NewCallbackButtonWithStyle(btnPM, "set_toggle_pm", markup.StyleBlue),
		markup.NewCallbackButtonWithStyle(btnBatch, "set_toggle_batch", markup.StyleBlue),
	})

	rows = append(rows, []tg.KeyboardButtonClass{
		markup.NewCallbackButtonWithStyle(fmt.Sprintf("⏱ TTL: %dh", dyn.TokenTTLHours), "set_ttl_step", markup.StyleGreen),
		markup.NewCallbackButtonWithStyle("🌐 Force-Sub Channels", "set_fsub_menu", markup.StyleGreen),
	})

	rows = append(rows, []tg.KeyboardButtonClass{
		markup.NewCallbackButtonWithStyle("🔄 Refresh", "set_refresh", markup.StyleBlue),
		markup.NewCallbackButtonWithStyle("❌ Close", "close", markup.StyleRed),
	})

	if msgID > 0 {
		return bm.editMessage(ctx, peer, msgID, text, markup.NewInlineMarkup(rows))
	}
	_, err := bm.sendTextWithMarkup(ctx, peer, text, markup.NewInlineMarkup(rows))
	return err
}

func (bm *BotManager) sendFSubSettingsPanel(ctx context.Context, peer tg.InputPeerClass) error {
	channels, _ := bm.database.ListFSubChannels(ctx)
	var sb strings.Builder
	sb.WriteString("⚙️ <b>Force-Subscription (FSub) Settings</b>\n\n")

	if len(channels) == 0 {
		sb.WriteString("<i>No channels currently monitored. Users can freely use the bot.</i>\n\n")
		sb.WriteString("To add a channel: <code>/set_fsub &lt;channel_id&gt; &lt;invite_link&gt;</code>")
	} else {
		sb.WriteString(fmt.Sprintf("<b>Active Monitored Channels (%d):</b>\n", len(channels)))
		for i, ch := range channels {
			sb.WriteString(fmt.Sprintf("%d. <code>%d</code> — <a href=\"%s\">Invite Link</a>\n", i+1, ch.ChannelID, ch.InviteURL))
		}
		sb.WriteString("\nTo remove a channel: <code>/rm_fsub &lt;channel_id&gt;</code>")
	}

	var rows [][]tg.KeyboardButtonClass
	for _, ch := range channels {
		rows = append(rows, []tg.KeyboardButtonClass{
			markup.NewCallbackButtonWithStyle(fmt.Sprintf("❌ Remove %d", ch.ChannelID), fmt.Sprintf("fsub_rm_%d", ch.ChannelID), markup.StyleRed),
		})
	}
	rows = append(rows, []tg.KeyboardButtonClass{
		markup.NewCallbackButtonWithStyle(markup.ToSmallCaps("Close"), "close", markup.StyleRed),
	})

	_, err := bm.sendTextWithMarkup(ctx, peer, sb.String(), markup.NewInlineMarkup(rows))
	return err
}

func runNetworkSpeedTest() (int64, float64) {
	testURL := "https://speed.cloudflare.com/__down?bytes=5000000" // 5MB download benchmark
	start := time.Now()

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Get(testURL)
	if err != nil {
		return 0, 0
	}
	defer resp.Body.Close()

	ping := time.Since(start).Milliseconds()

	downloadStart := time.Now()
	n, _ := io.Copy(io.Discard, resp.Body)
	duration := time.Since(downloadStart).Seconds()

	if duration <= 0 {
		duration = 0.1
	}

	speedMbps := (float64(n*8) / (1024 * 1024)) / duration
	return ping, speedMbps
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
