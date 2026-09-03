package bot

import (
	"context"
	"fmt"
	"time"

	"filetolink-go/internal/markup"

	"github.com/gotd/td/tg"
)

func (bm *BotManager) handleHelp(ctx context.Context, chatID int64) error {
	peer := toInputPeer(chatID)
	helpText := "📖 <b>Help Guide</b>\n\n" +
		"• <b>Private:</b> Send any media file directly to generate links.\n" +
		"• <b>Groups:</b> Reply to any file with <code>/link</code>.\n" +
		"• <b>Batch:</b> Reply with <code>/link 5</code> to process 5 consecutive files.\n" +
		"• <b>DC Info:</b> <code>/dc</code> or reply to a file/user to check Telegram Data Center.\n" +
		"• <b>Ping:</b> <code>/ping</code> to check bot latency."
	return bm.sendText(ctx, peer, helpText)
}

func (bm *BotManager) handleAbout(ctx context.Context, chatID int64) error {
	peer := toInputPeer(chatID)
	aboutText := "🌟 <b>FileToLink-Go Edition</b> 🌟\n\n" +
		"• <b>Developer:</b> @ExE_AQUIB\n" +
		"• <b>Updates Channel:</b> @Canon_Bots\n" +
		"• <b>Source Code:</b> <a href=\"https://github.com/aquib4040/filetolink-go\">filetolink-go</a>\n\n" +
		"🚀 <b>Performance & Architecture:</b>\n" +
		"• Written in <b>Go</b> for blazing fast download speeds and extreme concurrency\n" +
		"• Ultra-low memory footprint (~20-40 MB RAM), ideal for free and low-RAM cloud containers\n" +
		"• Multi-worker MTProto parallel chunk fetching for maximum bandwidth\n" +
		"• Zero-database stateless encrypted link tokens (AES-256-GCM)"

	var rows [][]tg.KeyboardButtonClass
	rows = append(rows, []tg.KeyboardButtonClass{
		markup.NewURLButtonWithStyle(markup.ToSmallCaps("Updates Channel"), "https://t.me/Canon_Bots", markup.StyleGreen),
		markup.NewURLButtonWithStyle(markup.ToSmallCaps("Developer"), "https://t.me/ExE_AQUIB", markup.StyleBlue),
	})
	rows = append(rows, []tg.KeyboardButtonClass{
		markup.NewURLButtonWithStyle(markup.ToSmallCaps("GitHub Repo"), "https://github.com/aquib4040/filetolink-go", markup.StyleBlue),
		markup.NewCallbackButtonWithStyle(markup.ToSmallCaps("Close"), "close", markup.StyleRed),
	})

	_, err := bm.sendTextWithMarkup(ctx, peer, aboutText, markup.NewInlineMarkup(rows))
	return err
}

func (bm *BotManager) handlePing(ctx context.Context, chatID int64) error {
	peer := toInputPeer(chatID)
	start := time.Now()
	msgID, err := bm.sendTextWithMarkup(ctx, peer, "🛰️ <b>Pinging...</b>", nil)
	if err == nil {
		elapsed := time.Since(start).Milliseconds()
		_ = bm.editMessage(ctx, peer, msgID, fmt.Sprintf("☁️ <b>PONG!</b> <code>%d ms</code>\n🤖 <b>Status:</b> <code>Active</code>", elapsed), nil)
	}
	return err
}
