package bot

import (
	"context"

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
		return bm.handleStart(ctx, senderID, chatID, isPrivate, args)

	case "/help":
		return bm.handleHelp(ctx, chatID)

	case "/about":
		return bm.handleAbout(ctx, chatID)

	case "/ping":
		return bm.handlePing(ctx, chatID)

	case "/dc":
		return bm.handleDC(ctx, msg, senderID, chatID, args)

	case "/speedtest":
		if senderID != bm.cfg.OwnerID {
			return bm.sendText(ctx, peer, "❌ Unauthorized.")
		}
		return bm.handleSpeedtest(ctx, chatID)

	case "/link":
		return bm.handleLinkCommand(ctx, msg, senderID, chatID, args)

	case "/batch":
		return bm.handleBatchCommand(ctx, chatID)

	// Admin / Settings Commands
	case "/settings":
		if senderID != bm.cfg.OwnerID {
			return bm.sendText(ctx, peer, "❌ Unauthorized.")
		}
		return bm.sendMainSettingsPanel(ctx, peer, 0)

	case "/set_shortener":
		if senderID != bm.cfg.OwnerID {
			return bm.sendText(ctx, peer, "❌ Unauthorized.")
		}
		return bm.handleSetShortener(ctx, chatID, args)

	case "/set_ttl":
		if senderID != bm.cfg.OwnerID {
			return bm.sendText(ctx, peer, "❌ Unauthorized.")
		}
		return bm.handleSetTTL(ctx, chatID, args)

	case "/fsub":
		if senderID != bm.cfg.OwnerID {
			return bm.sendText(ctx, peer, "❌ Unauthorized.")
		}
		return bm.sendFSubSettingsPanel(ctx, peer)

	case "/set_fsub":
		if senderID != bm.cfg.OwnerID {
			return bm.sendText(ctx, peer, "❌ Unauthorized.")
		}
		return bm.handleSetFSub(ctx, chatID, args)

	case "/rm_fsub":
		if senderID != bm.cfg.OwnerID {
			return bm.sendText(ctx, peer, "❌ Unauthorized.")
		}
		return bm.handleRmFSub(ctx, chatID, args)

	case "/status":
		if senderID != bm.cfg.OwnerID {
			return bm.sendText(ctx, peer, "❌ Unauthorized.")
		}
		return bm.handleStatus(ctx, chatID)

	case "/users":
		if senderID != bm.cfg.OwnerID {
			return bm.sendText(ctx, peer, "❌ Unauthorized.")
		}
		return bm.handleUsers(ctx, chatID)

	case "/stats":
		if senderID != bm.cfg.OwnerID {
			return bm.sendText(ctx, peer, "❌ Unauthorized.")
		}
		return bm.handleStats(ctx, chatID)

	case "/ban":
		if senderID != bm.cfg.OwnerID {
			return bm.sendText(ctx, peer, "❌ Unauthorized.")
		}
		return bm.handleBan(ctx, chatID, args)

	case "/unban":
		if senderID != bm.cfg.OwnerID {
			return bm.sendText(ctx, peer, "❌ Unauthorized.")
		}
		return bm.handleUnban(ctx, chatID, args)

	case "/listbanned":
		if senderID != bm.cfg.OwnerID {
			return bm.sendText(ctx, peer, "❌ Unauthorized.")
		}
		return bm.handleListBanned(ctx, chatID)

	case "/auth_gc", "/authorize_gc":
		if senderID != bm.cfg.OwnerID {
			return bm.sendText(ctx, peer, "❌ Unauthorized.")
		}
		return bm.handleAuthGC(ctx, senderID, chatID, args)

	case "/deauth_gc":
		if senderID != bm.cfg.OwnerID {
			return bm.sendText(ctx, peer, "❌ Unauthorized.")
		}
		return bm.handleDeauthGC(ctx, chatID, args)

	case "/listauth_gc":
		if senderID != bm.cfg.OwnerID {
			return bm.sendText(ctx, peer, "❌ Unauthorized.")
		}
		return bm.handleListAuthGC(ctx, chatID)

	case "/addpaid":
		if senderID != bm.cfg.OwnerID {
			return bm.sendText(ctx, peer, "❌ Unauthorized.")
		}
		return bm.handleAddPaid(ctx, senderID, chatID, args)

	case "/removepaid":
		if senderID != bm.cfg.OwnerID {
			return bm.sendText(ctx, peer, "❌ Unauthorized.")
		}
		return bm.handleRemovePaid(ctx, chatID, args)

	case "/listpaid":
		if senderID != bm.cfg.OwnerID {
			return bm.sendText(ctx, peer, "❌ Unauthorized.")
		}
		return bm.handleListPaid(ctx, chatID)

	case "/pmmode":
		if senderID != bm.cfg.OwnerID {
			return bm.sendText(ctx, peer, "❌ Unauthorized.")
		}
		return bm.handlePMMode(ctx, chatID, args)

	case "/restart":
		if senderID != bm.cfg.OwnerID {
			return bm.sendText(ctx, peer, "❌ Unauthorized.")
		}
		return bm.handleRestart(ctx, chatID)
	}

	return nil
}
