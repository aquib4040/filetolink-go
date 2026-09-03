package bot

import (
	"context"
	"fmt"
	"time"
)

func (bm *BotManager) handleHelp(ctx context.Context, chatID int64) error {
	peer := toInputPeer(chatID)
	helpText := "📖 <b>Help Guide</b>\n\n" +
		"• <b>Private:</b> Send any media file directly to generate links.\n" +
		"• <b>Groups:</b> Reply to any file with <code>/link</code>.\n" +
		"• <b>Batch:</b> Reply with <code>/link 5</code> to process 5 consecutive files."
	return bm.sendText(ctx, peer, helpText)
}

func (bm *BotManager) handleAbout(ctx context.Context, chatID int64) error {
	peer := toInputPeer(chatID)
	aboutText := "🌟 <b>FileToLink Go Edition</b>\n\n" +
		"• High-throughput Go MTProto streaming service\n" +
		"• Zero-database stateless encrypted link architecture\n" +
		"• Built with ❤️ for maximum streaming performance"
	return bm.sendText(ctx, peer, aboutText)
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
