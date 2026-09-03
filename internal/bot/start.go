package bot

import (
	"context"
	"fmt"
	"strings"

	"filetolink-go/internal/markup"

	"github.com/gotd/td/tg"
)

func (bm *BotManager) handleStart(ctx context.Context, senderID, chatID int64, isPrivate bool, args []string) error {
	peer := toInputPeer(chatID)

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
}

func (bm *BotManager) sendStartInDMPrompt(ctx context.Context, peer tg.InputPeerClass, replyToID int) {
	botUsername := "bot"
	if bm.botUser != nil && bm.botUser.Username != "" {
		botUsername = bm.botUser.Username
	}
	startURL := fmt.Sprintf("https://t.me/%s?start=true", botUsername)

	var rows [][]tg.KeyboardButtonClass
	rows = append(rows, []tg.KeyboardButtonClass{
		markup.NewURLButtonWithStyle(markup.ToSmallCaps("Start In DM"), startURL, markup.StyleGreen),
	})

	text := "👋 <b>Please start me in PM first!</b>\n\nClick the button below to start the bot in private messages before requesting links."
	_, _ = bm.sendTextWithMarkup(ctx, peer, text, markup.NewInlineMarkup(rows))
}
