package bot

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"

	"filetolink-go/internal/pool"

	"github.com/gotd/td/tg"
)

func isStreamable(fileName string) bool {
	ext := strings.ToLower(filepath.Ext(fileName))
	switch ext {
	case ".mp4", ".mkv", ".webm", ".avi", ".mov", ".flv", ".wmv", ".m4v", ".ts",
		".mp3", ".m4a", ".flac", ".wav", ".ogg", ".opus", ".aac":
		return true
	default:
		return false
	}
}

func extractMediaInfo(msg *tg.Message) (fileName string, fileSize int64, fileHash string) {
	if msg == nil || msg.Media == nil {
		return "file.bin", 0, ""
	}

	switch m := msg.Media.(type) {
	case *tg.MessageMediaDocument:
		if doc, ok := m.Document.(*tg.Document); ok {
			fileSize = doc.Size
			fileName = fmt.Sprintf("file_%d.bin", doc.ID)
			for _, attr := range doc.Attributes {
				if fn, ok := attr.(*tg.DocumentAttributeFilename); ok {
					fileName = fn.FileName
					break
				}
			}
			h := sha256.Sum256([]byte(fmt.Sprintf("%d:%d", doc.ID, doc.AccessHash)))
			fileHash = hex.EncodeToString(h[:])
			return
		}
	case *tg.MessageMediaPhoto:
		if photo, ok := m.Photo.(*tg.Photo); ok {
			fileName = fmt.Sprintf("photo_%d.jpg", photo.ID)
			fileSize = 1024 * 1024 // estimate
			h := sha256.Sum256([]byte(fmt.Sprintf("%d:%d", photo.ID, photo.AccessHash)))
			fileHash = hex.EncodeToString(h[:])
			return
		}
	}
	return "file.bin", 0, ""
}

func extractMsgID(res tg.UpdatesClass) int {
	switch u := res.(type) {
	case *tg.Updates:
		for _, upd := range u.Updates {
			if m, ok := upd.(*tg.UpdateMessageID); ok {
				return m.ID
			}
			if nm, ok := upd.(*tg.UpdateNewChannelMessage); ok {
				if msg, ok := nm.Message.(*tg.Message); ok {
					return msg.ID
				}
			}
			if nm, ok := upd.(*tg.UpdateNewMessage); ok {
				if msg, ok := nm.Message.(*tg.Message); ok {
					return msg.ID
				}
			}
		}
	case *tg.UpdateShortSentMessage:
		return u.ID
	}
	return 0
}

func extractMessagesFromClass(res tg.MessagesMessagesClass) []*tg.Message {
	var out []*tg.Message
	switch m := res.(type) {
	case *tg.MessagesMessages:
		for _, msgClass := range m.Messages {
			if msg, ok := msgClass.(*tg.Message); ok {
				out = append(out, msg)
			}
		}
	case *tg.MessagesMessagesSlice:
		for _, msgClass := range m.Messages {
			if msg, ok := msgClass.(*tg.Message); ok {
				out = append(out, msg)
			}
		}
	case *tg.MessagesChannelMessages:
		for _, msgClass := range m.Messages {
			if msg, ok := msgClass.(*tg.Message); ok {
				out = append(out, msg)
			}
		}
	}
	return out
}

func humanBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.2f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}

func toInputChannel(chatID int64) tg.InputChannelClass {
	raw := pool.RawChannelID(chatID)
	return &tg.InputChannel{ChannelID: raw}
}
