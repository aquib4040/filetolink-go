package bot

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"
)

func (bm *BotManager) handleSpeedtest(ctx context.Context, chatID int64) error {
	peer := toInputPeer(chatID)
	statusMsgID, err := bm.sendTextWithMarkup(ctx, peer, "⚡ <b>Running Network Speed Benchmark...</b>\n\nTesting ping & CDN download bandwidth...", nil)
	if err != nil {
		return err
	}

	startPing := time.Now()
	testURL := "https://speed.cloudflare.com/__down?bytes=10485760" // 10MB test
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, testURL, nil)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		_ = bm.editMessage(ctx, peer, statusMsgID, fmt.Sprintf("❌ <b>Speedtest Failed:</b> %v", err), nil)
		return err
	}
	defer resp.Body.Close()

	pingMs := time.Since(startPing).Milliseconds()

	startDownload := time.Now()
	n, err := io.Copy(io.Discard, resp.Body)
	if err != nil {
		_ = bm.editMessage(ctx, peer, statusMsgID, fmt.Sprintf("❌ <b>Speedtest read error:</b> %v", err), nil)
		return err
	}
	duration := time.Since(startDownload).Seconds()
	if duration <= 0 {
		duration = 0.001
	}

	mbps := (float64(n*8) / (1024 * 1024)) / duration

	resText := fmt.Sprintf("⚡ <b>Speedtest Results</b> ⚡\n\n"+
		"• <b>Ping / Latency:</b> <code>%d ms</code>\n"+
		"• <b>Transferred:</b> <code>%.2f MB</code>\n"+
		"• <b>Download Speed:</b> <code>%.2f Mbps</code>\n"+
		"• <b>Test Server:</b> <code>Cloudflare CDN Edge</code>",
		pingMs, float64(n)/(1024*1024), mbps)

	return bm.editMessage(ctx, peer, statusMsgID, resText, nil)
}
