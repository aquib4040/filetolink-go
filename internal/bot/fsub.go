package bot

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"filetolink-go/internal/db"
	"filetolink-go/internal/markup"
	"filetolink-go/internal/pool"

	"github.com/gotd/td/tg"
)

func (bm *BotManager) checkFSub(ctx context.Context, userID int64, peer tg.InputPeerClass) bool {
	if userID == bm.cfg.OwnerID || bm.database.IsPremiumUser(ctx, userID) {
		return true
	}

	channels, err := bm.database.ListFSubChannels(ctx)
	if err != nil || len(channels) == 0 {
		return true
	}

	var missing []db.FSubChannel
	for _, ch := range channels {
		p := &tg.InputChannel{
			ChannelID: pool.RawChannelID(ch.ChannelID),
		}
		part, err := bm.api.ChannelsGetParticipant(ctx, &tg.ChannelsGetParticipantRequest{
			Channel:     p,
			Participant: &tg.InputPeerUser{UserID: userID},
		})
		if err != nil || part == nil {
			missing = append(missing, ch)
		}
	}

	if len(missing) == 0 {
		return true
	}

	var rows [][]tg.KeyboardButtonClass
	for i, ch := range missing {
		btnText := fmt.Sprintf("📢 Join Channel %d", i+1)
		rows = append(rows, []tg.KeyboardButtonClass{
			markup.NewURLButtonWithStyle(btnText, ch.InviteURL, markup.StyleGreen),
		})
	}
	rows = append(rows, []tg.KeyboardButtonClass{
		markup.NewCallbackButtonWithStyle("🔄 Try Again", "close", markup.StyleBlue),
	})

	text := "⚠️ <b>Please Join Our Channels to Use This Bot!</b>\n\nYou must be a subscriber of the following channels to generate links:"
	_, _ = bm.sendTextWithMarkup(ctx, peer, text, markup.NewInlineMarkup(rows))
	return false
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
		markup.NewCallbackButtonWithStyle("❌ Close", "close", markup.StyleRed),
	})

	_, err := bm.sendTextWithMarkup(ctx, peer, sb.String(), markup.NewInlineMarkup(rows))
	return err
}

func (bm *BotManager) handleSetFSub(ctx context.Context, chatID int64, args []string) error {
	peer := toInputPeer(chatID)
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
}

func (bm *BotManager) handleRmFSub(ctx context.Context, chatID int64, args []string) error {
	peer := toInputPeer(chatID)
	if len(args) < 1 {
		return bm.sendText(ctx, peer, "⚠️ <b>Usage:</b> <code>/rm_fsub &lt;channel_id&gt;</code>")
	}
	chID, _ := strconv.ParseInt(args[0], 10, 64)
	_ = bm.database.RemoveFSubChannel(ctx, chID)
	return bm.sendText(ctx, peer, fmt.Sprintf("✅ Force-Sub channel <code>%d</code> removed.", chID))
}
