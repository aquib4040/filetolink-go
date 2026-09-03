package bot

import (
	"context"
	"fmt"
	"time"

	"github.com/showwin/speedtest-go/speedtest"
)

func (bm *BotManager) handleSpeedtest(ctx context.Context, chatID int64) error {
	peer := toInputPeer(chatID)
	statusMsgID, err := bm.sendTextWithMarkup(ctx, peer, "⚡ <i>Initiating Speedtest...</i>", nil)
	if err != nil {
		return err
	}

	go func() {
		// 1. Fetch user & network info
		user, err := speedtest.FetchUserInfo()
		if err != nil {
			_ = bm.editMessage(ctx, peer, statusMsgID, "<b>ERROR:</b> <i>Can't connect to Speedtest Server at the moment, try again later!</i>", nil)
			return
		}

		_ = bm.editMessage(ctx, peer, statusMsgID, "⚡ <i>Finding best Speedtest server...</i>", nil)

		// 2. Fetch server list and pick closest server
		serverList, err := speedtest.FetchServers()
		if err != nil || len(serverList) == 0 {
			_ = bm.editMessage(ctx, peer, statusMsgID, "<b>ERROR:</b> <i>Failed to fetch speedtest servers list.</i>", nil)
			return
		}

		targets, err := serverList.FindServer([]int{})
		if err != nil || len(targets) == 0 {
			_ = bm.editMessage(ctx, peer, statusMsgID, "<b>ERROR:</b> <i>Could not find available speedtest servers.</i>", nil)
			return
		}

		server := targets[0]

		_ = bm.editMessage(ctx, peer, statusMsgID, fmt.Sprintf("⚡ <i>Running Ping & Download test on <b>%s (%s)</b>...</i>", server.Name, server.Country), nil)

		// 3. Ping Test
		_ = server.PingTest(nil)

		// 4. Download Test
		_ = server.DownloadTest()

		_ = bm.editMessage(ctx, peer, statusMsgID, fmt.Sprintf("⚡ <i>Running Upload test on <b>%s</b>...</i>", server.Name), nil)

		// 5. Upload Test
		_ = server.UploadTest()

		// Calculations (DLSpeed & ULSpeed are in Byte/s, * 8 for bps or / (1024*1024) for MB/s)
		dlMBs := float64(server.DLSpeed) / (1024 * 1024)
		ulMBs := float64(server.ULSpeed) / (1024 * 1024)
		dlMbps := (float64(server.DLSpeed) * 8) / (1000 * 1000)
		ulMbps := (float64(server.ULSpeed) * 8) / (1000 * 1000)

		nowStr := time.Now().UTC().Format("2006-01-02 15:04:05 UTC")

		isp := user.Isp
		if isp == "" {
			isp = "Unknown"
		}
		ip := user.IP
		if ip == "" {
			ip = "Hidden"
		}

		resultText := fmt.Sprintf(`⚡ <b><i>SPEEDTEST INFO</i></b>
• <b>Upload:</b> <code>%.2f MB/s</code> (<code>%.2f Mbps</code>)
• <b>Download:</b> <code>%.2f MB/s</code> (<code>%.2f Mbps</code>)
• <b>Ping:</b> <code>%d ms</code>
• <b>Jitter:</b> <code>%d ms</code>
• <b>Time:</b> <code>%s</code>

🌐 <b><i>SPEEDTEST SERVER</i></b>
• <b>Name:</b> <code>%s</code>
• <b>Country:</b> <code>%s</code>
• <b>Sponsor:</b> <code>%s</code>
• <b>Latency:</b> <code>%s</code>

🖥 <b><i>CLIENT DETAILS</i></b>
• <b>IP Address:</b> <code>%s</code>
• <b>ISP:</b> <code>%s</code>`,
			ulMBs, ulMbps,
			dlMBs, dlMbps,
			server.Latency.Milliseconds(),
			server.Jitter.Milliseconds(),
			nowStr,
			server.Name,
			server.Country,
			server.Sponsor,
			server.Latency.String(),
			ip,
			isp,
		)

		_ = bm.editMessage(ctx, peer, statusMsgID, resultText, nil)
	}()

	return nil
}
