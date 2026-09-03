package bot

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func (bm *BotManager) handleStatus(ctx context.Context, chatID int64) error {
	peer := toInputPeer(chatID)
	workloads := bm.pool.GetWorkloads()
	uptime := time.Since(bm.uptime).Round(time.Second)

	totalActive := int32(0)
	for _, w := range workloads {
		totalActive += w.ActiveStreams
	}

	var sb strings.Builder
	sb.WriteString("🌐 <b>System Status:</b> <code>Operational</code>\n\n")
	sb.WriteString(fmt.Sprintf("⏱ <b>Uptime:</b> <code>%s</code>\n", uptime))
	sb.WriteString(fmt.Sprintf("🤖 <b>Bot Instances:</b> <code>%d</code>\n", len(workloads)))
	sb.WriteString(fmt.Sprintf("⚡ <b>Total Workload:</b> <code>%d active streams</code>\n\n", totalActive))
	sb.WriteString("📊 <b>Workload Distribution:</b>\n\n")

	if len(workloads) == 0 {
		sb.WriteString("<i>No worker bot sessions configured.</i>\n")
	} else {
		for _, w := range workloads {
			statusTag := "ONLINE"
			if w.IsInvalid {
				statusTag = "INVALID / SKIPPED"
			} else if !w.IsReady {
				statusTag = "CONNECTING"
			}

			sb.WriteString(fmt.Sprintf("• <b>Client %d</b> (<code>...%s</code>): <b>%d</b> active connections (Total: %d) [<code>%s</code>]\n",
				w.Index+1, w.TokenSuffix, w.ActiveStreams, w.TotalStreams, statusTag))
		}
	}

	return bm.sendText(ctx, peer, sb.String())
}

func (bm *BotManager) handleUsers(ctx context.Context, chatID int64) error {
	peer := toInputPeer(chatID)
	total := bm.database.TotalUsers(ctx)
	text := fmt.Sprintf("👥 <b>Total Users:</b> <code>%d</code>", total)
	return bm.sendText(ctx, peer, text)
}

func (bm *BotManager) handleStats(ctx context.Context, chatID int64) error {
	peer := toInputPeer(chatID)
	stats, _ := bm.database.GetTrafficStats(ctx)
	activeSess := bm.pool.ActiveSessionCount()
	uptime := time.Since(bm.uptime).Round(time.Second)

	text := fmt.Sprintf("📊 <b>System & Streaming Analytics</b> 📊\n\n"+
		"• <b>Bot Uptime:</b> <code>%s</code>\n"+
		"• <b>Active MTProto Pool:</b> <code>%d workers</code>\n\n"+
		"📈 <b>Transferred Bandwidth:</b>\n"+
		"• <b>Today:</b> <code>%s</code>\n"+
		"• <b>This Week:</b> <code>%s</code>\n"+
		"• <b>This Month:</b> <code>%s</code>\n"+
		"• <b>This Year:</b> <code>%s</code>\n"+
		"• <b>Overall:</b> <code>%s</code>",
		uptime, activeSess,
		humanBytes(stats.Today),
		humanBytes(stats.ThisWeek),
		humanBytes(stats.ThisMonth),
		humanBytes(stats.ThisYear),
		humanBytes(stats.Overall),
	)
	return bm.sendText(ctx, peer, text)
}

func (bm *BotManager) handleBan(ctx context.Context, chatID int64, args []string) error {
	peer := toInputPeer(chatID)
	if len(args) < 1 {
		return bm.sendText(ctx, peer, "⚠️ <b>Usage:</b> <code>/ban &lt;user_id&gt; [reason]</code>")
	}
	targetUID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil || targetUID == 0 {
		return bm.sendText(ctx, peer, "❌ Invalid user ID.")
	}
	reason := "Banned by admin"
	if len(args) > 1 {
		reason = strings.Join(args[1:], " ")
	}
	_ = bm.database.BanUser(ctx, targetUID, reason)
	return bm.sendText(ctx, peer, fmt.Sprintf("⛔ User <code>%d</code> has been banned.\nReason: <i>%s</i>", targetUID, reason))
}

func (bm *BotManager) handleUnban(ctx context.Context, chatID int64, args []string) error {
	peer := toInputPeer(chatID)
	if len(args) < 1 {
		return bm.sendText(ctx, peer, "⚠️ <b>Usage:</b> <code>/unban &lt;user_id&gt;</code>")
	}
	targetUID, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil || targetUID == 0 {
		return bm.sendText(ctx, peer, "❌ Invalid user ID.")
	}
	_ = bm.database.UnbanUser(ctx, targetUID)
	return bm.sendText(ctx, peer, fmt.Sprintf("✅ User <code>%d</code> has been unbanned.", targetUID))
}

func (bm *BotManager) handleListBanned(ctx context.Context, chatID int64) error {
	peer := toInputPeer(chatID)
	banned, err := bm.database.ListBannedUsers(ctx)
	if err != nil || len(banned) == 0 {
		return bm.sendText(ctx, peer, "ℹ️ No users are currently banned.")
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("⛔ <b>Banned Users (%d):</b>\n\n", len(banned)))
	for i, b := range banned {
		sb.WriteString(fmt.Sprintf("%d. <code>%d</code> — Reason: <i>%s</i> (%s)\n",
			i+1, b.UserID, b.Reason, b.BannedAt.Format("02 Jan 2006")))
	}
	return bm.sendText(ctx, peer, sb.String())
}

func (bm *BotManager) handleAuthGC(ctx context.Context, senderID, chatID int64, args []string) error {
	peer := toInputPeer(chatID)
	targetID := chatID
	if len(args) > 0 {
		if id, err := strconv.ParseInt(args[0], 10, 64); err == nil {
			targetID = id
		}
	}
	_ = bm.database.AuthorizeGC(ctx, targetID, senderID)
	return bm.sendText(ctx, peer, fmt.Sprintf("✅ Group <code>%d</code> authorized for link generation.", targetID))
}

func (bm *BotManager) handleDeauthGC(ctx context.Context, chatID int64, args []string) error {
	peer := toInputPeer(chatID)
	targetID := chatID
	if len(args) > 0 {
		if id, err := strconv.ParseInt(args[0], 10, 64); err == nil {
			targetID = id
		}
	}
	_ = bm.database.DeauthorizeGC(ctx, targetID)
	return bm.sendText(ctx, peer, fmt.Sprintf("🚫 Group <code>%d</code> deauthorized.", targetID))
}

func (bm *BotManager) handleListAuthGC(ctx context.Context, chatID int64) error {
	peer := toInputPeer(chatID)
	list, _ := bm.database.ListAuthorizedGCs(ctx)
	if len(list) == 0 {
		return bm.sendText(ctx, peer, "ℹ️ No groups currently authorized.")
	}
	var sb strings.Builder
	sb.WriteString("<b>Authorized Groups:</b>\n")
	for _, g := range list {
		sb.WriteString(fmt.Sprintf("• <code>%d</code>\n", g.ChatID))
	}
	return bm.sendText(ctx, peer, sb.String())
}

func (bm *BotManager) handleAddPaid(ctx context.Context, senderID, chatID int64, args []string) error {
	peer := toInputPeer(chatID)
	if len(args) < 2 {
		return bm.sendText(ctx, peer, "⚠️ <b>Usage:</b> <code>/addpaid &lt;user_id&gt; &lt;duration&gt;</code>\nExample durations: <code>30d</code>, <code>1m</code>, <code>12h</code>, <code>3600s</code>")
	}
	targetUID, _ := strconv.ParseInt(args[0], 10, 64)
	dur, err := ParseDurationString(args[1])
	if err != nil {
		return bm.sendText(ctx, peer, fmt.Sprintf("❌ Invalid duration format: %v", err))
	}
	_ = bm.database.AddPremium(ctx, targetUID, time.Now().Add(dur), senderID)
	return bm.sendText(ctx, peer, fmt.Sprintf("💎 User <code>%d</code> granted Paid status for <b>%s</b>.", targetUID, args[1]))
}

func (bm *BotManager) handleRemovePaid(ctx context.Context, chatID int64, args []string) error {
	peer := toInputPeer(chatID)
	if len(args) < 1 {
		return bm.sendText(ctx, peer, "⚠️ <b>Usage:</b> <code>/removepaid &lt;user_id&gt;</code>")
	}
	targetUID, _ := strconv.ParseInt(args[0], 10, 64)
	_ = bm.database.RemovePremium(ctx, targetUID)
	return bm.sendText(ctx, peer, fmt.Sprintf("🚫 User <code>%d</code> removed from Paid subscribers.", targetUID))
}

func (bm *BotManager) handleListPaid(ctx context.Context, chatID int64) error {
	peer := toInputPeer(chatID)
	list, _ := bm.database.ListPremiumUsers(ctx)
	if len(list) == 0 {
		return bm.sendText(ctx, peer, "ℹ️ No active paid subscribers.")
	}
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("💎 <b>Active Paid Subscribers (%d):</b>\n\n", len(list)))
	for i, u := range list {
		sb.WriteString(fmt.Sprintf("%d. <code>%d</code> — Expires: <b>%s</b>\n",
			i+1, u.UserID, u.ExpiresAt.Format("02 Jan 2006 15:04 MST")))
	}
	return bm.sendText(ctx, peer, sb.String())
}

func (bm *BotManager) handlePMMode(ctx context.Context, chatID int64, args []string) error {
	peer := toInputPeer(chatID)
	if len(args) < 1 {
		return bm.sendText(ctx, peer, "⚠️ <b>Usage:</b> <code>/pmmode &lt;on|off&gt;</code>")
	}
	mode := strings.ToLower(args[0]) == "on"
	_ = bm.UpdateSetting(ctx, "pm_mode", mode)
	statusStr := "DISABLED (Restricted to paid users)"
	if mode {
		statusStr = "ENABLED (Open to all users in PM)"
	}
	return bm.sendText(ctx, peer, fmt.Sprintf("⚙️ PM Link Generation is now <b>%s</b>.", statusStr))
}

func (bm *BotManager) handleRestart(ctx context.Context, chatID int64) error {
	peer := toInputPeer(chatID)
	msgID, err := bm.sendTextWithMarkup(ctx, peer, "🔄 <b>Restarting streaming engine...</b>\n<i>Please wait a few seconds...</i>", nil)
	if err == nil {
		_ = bm.database.SaveRestartMessage(ctx, int64(msgID), chatID)
	}

	go func() {
		time.Sleep(1 * time.Second)
		os.Exit(0)
	}()
	return nil
}
