package bot

import (
	"context"
	"fmt"
	"strconv"

	"filetolink-go/internal/markup"

	"github.com/gotd/td/tg"
)

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

func (bm *BotManager) handleSetShortener(ctx context.Context, chatID int64, args []string) error {
	peer := toInputPeer(chatID)
	if len(args) < 2 {
		return bm.sendText(ctx, peer, "⚠️ <b>Usage:</b> <code>/set_shortener &lt;site&gt; &lt;api_key&gt;</code>")
	}
	_ = bm.UpdateSetting(ctx, "shortener_site", args[0])
	_ = bm.UpdateSetting(ctx, "shortener_api_key", args[1])
	return bm.sendText(ctx, peer, fmt.Sprintf("✅ URL Shortener updated:\nSite: <code>%s</code>\nKey: <code>%s...</code>", args[0], args[1][:min(len(args[1]), 6)]))
}

func (bm *BotManager) handleSetTTL(ctx context.Context, chatID int64, args []string) error {
	peer := toInputPeer(chatID)
	if len(args) < 1 {
		return bm.sendText(ctx, peer, "⚠️ <b>Usage:</b> <code>/set_ttl &lt;hours&gt;</code>")
	}
	hrs, err := strconv.Atoi(args[0])
	if err != nil || hrs <= 0 {
		return bm.sendText(ctx, peer, "❌ Invalid hours value.")
	}
	_ = bm.UpdateSetting(ctx, "token_ttl_hours", hrs)
	return bm.sendText(ctx, peer, fmt.Sprintf("✅ Token validity TTL updated to: <b>%d Hours</b>", hrs))
}
