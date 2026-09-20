package bot

import (
	"testing"

	"github.com/gotd/td/tg"
)

func TestExtractMsgID(t *testing.T) {
	// Case 1: Updates with UpdateMessageID (source ID 108948) AND UpdateNewChannelMessage (dest ID 2000)
	// Must return 2000 (the forwarded message ID in BIN_CHANNEL), NOT the source ID.
	upds := &tg.Updates{
		Updates: []tg.UpdateClass{
			&tg.UpdateMessageID{ID: 108948, RandomID: 123},
			&tg.UpdateNewChannelMessage{
				Message: &tg.Message{
					ID: 2000,
				},
			},
		},
	}
	id := extractMsgID(upds)
	if id != 2000 {
		t.Fatalf("expected 2000, got %d", id)
	}

	// Case 2: UpdatesCombined with UpdateMessageID and UpdateNewChannelMessage
	updsCombined := &tg.UpdatesCombined{
		Updates: []tg.UpdateClass{
			&tg.UpdateMessageID{ID: 108948, RandomID: 123},
			&tg.UpdateNewChannelMessage{
				Message: &tg.Message{
					ID: 3000,
				},
			},
		},
	}
	id = extractMsgID(updsCombined)
	if id != 3000 {
		t.Fatalf("expected 3000, got %d", id)
	}

	// Case 3: Direct chat message (UpdateNewMessage)
	updDirect := &tg.Updates{
		Updates: []tg.UpdateClass{
			&tg.UpdateNewMessage{
				Message: &tg.Message{
					ID: 3500,
				},
			},
		},
	}
	id = extractMsgID(updDirect)
	if id != 3500 {
		t.Fatalf("expected 3500, got %d", id)
	}

	// Case 4: Fallback to UpdateMessageID when no NewMessage is available
	fallback := &tg.Updates{
		Updates: []tg.UpdateClass{
			&tg.UpdateMessageID{ID: 4000, RandomID: 123},
		},
	}
	id = extractMsgID(fallback)
	if id != 4000 {
		t.Fatalf("expected 4000, got %d", id)
	}

	// Case 5: UpdateShortSentMessage
	short := &tg.UpdateShortSentMessage{ID: 5000}
	id = extractMsgID(short)
	if id != 5000 {
		t.Fatalf("expected 5000, got %d", id)
	}
}
