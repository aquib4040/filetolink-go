package bot

import (
	"context"

	"filetolink-go/internal/pool"

	"github.com/gotd/td/tg"
)

func (bm *BotManager) checkReelMedia(ctx context.Context, msg *tg.Message, peerID int64) {
	if bm.cfg.ReelChannelID == 0 || peerID != pool.FormatChannelID(bm.cfg.ReelChannelID) || msg.Media == nil {
		return
	}

	fileType := "video"
	if _, ok := msg.Media.(*tg.MessageMediaPhoto); ok {
		fileType = "photo"
	}

	_ = bm.database.SaveReelMedia(ctx, msg.ID, fileType)
}
