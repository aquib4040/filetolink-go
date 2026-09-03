package markup

import (
	"strings"

	"github.com/gotd/td/tg"
)

var (
	StyleGreen = tg.KeyboardButtonStyle{BgSuccess: true}
	StyleRed   = tg.KeyboardButtonStyle{BgDanger: true}
	StyleBlue  = tg.KeyboardButtonStyle{BgPrimary: true}
)

func NewInlineMarkup(rows [][]tg.KeyboardButtonClass) *tg.ReplyInlineMarkup {
	var kbRows []tg.KeyboardButtonRow
	for _, row := range rows {
		var buttons []tg.KeyboardButtonClass
		buttons = append(buttons, row...)
		kbRows = append(kbRows, tg.KeyboardButtonRow{Buttons: buttons})
	}
	return &tg.ReplyInlineMarkup{Rows: kbRows}
}

func NewCallbackButton(text, data string) *tg.KeyboardButtonCallback {
	return &tg.KeyboardButtonCallback{
		Text: text,
		Data: []byte(data),
	}
}

func NewCallbackButtonWithStyle(text, data string, style tg.KeyboardButtonStyle) *tg.KeyboardButtonCallback {
	btn := &tg.KeyboardButtonCallback{
		Text: text,
		Data: []byte(data),
	}
	btn.SetStyle(style)
	return btn
}

func NewURLButton(text, url string) *tg.KeyboardButtonURL {
	return &tg.KeyboardButtonURL{
		Text: text,
		URL:  url,
	}
}

func NewURLButtonWithStyle(text, url string, style tg.KeyboardButtonStyle) *tg.KeyboardButtonURL {
	btn := &tg.KeyboardButtonURL{
		Text: text,
		URL:  url,
	}
	btn.SetStyle(style)
	return btn
}

// ToSmallCaps converts standard ASCII letters to unicode small caps
func ToSmallCaps(s string) string {
	mapping := map[rune]rune{
		'a': 'ᴀ', 'b': 'ʙ', 'c': 'ᴄ', 'd': 'ᴅ', 'e': 'ᴇ', 'f': 'ғ', 'g': 'ɢ', 'h': 'ʜ',
		'i': 'ɪ', 'j': 'ᴊ', 'k': 'ᴋ', 'l': 'ʟ', 'm': 'ᴍ', 'n': 'ɴ', 'o': 'ᴏ', 'p': 'ᴘ',
		'q': 'ǫ', 'r': 'ʀ', 's': 's', 't': 'ᴛ', 'u': 'ᴜ', 'v': 'ᴠ', 'w': 'ᴡ', 'x': 'x',
		'y': 'ʏ', 'z': 'ᴢ',
		'A': 'ᴀ', 'B': 'ʙ', 'C': 'ᴄ', 'D': 'ᴅ', 'E': 'ᴇ', 'F': 'ғ', 'G': 'ɢ', 'H': 'ʜ',
		'I': 'ɪ', 'J': 'ᴊ', 'K': 'ᴋ', 'L': 'ʟ', 'M': 'ᴍ', 'N': 'ɴ', 'O': 'ᴏ', 'P': 'ᴘ',
		'Q': 'ǫ', 'R': 'ʀ', 'S': 's', 'T': 'ᴛ', 'U': 'ᴜ', 'V': 'ᴠ', 'W': 'ᴡ', 'X': 'x',
		'Y': 'ʏ', 'Z': 'ᴢ',
	}
	var sb strings.Builder
	for _, r := range s {
		if sc, ok := mapping[r]; ok {
			sb.WriteRune(sc)
		} else {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}
