package channels

import "testing"

func TestTextMentionIndex(t *testing.T) {
	tests := []struct {
		name    string
		content string
		token   string
		want    int
	}{
		{"empty token", "hello bot", "", -1},
		{"exact word", "hello bot", "bot", 6},
		{"case insensitive", "hello BOT", "bot", 6},
		{"punctuation after", "bot, hello", "bot", 0},
		{"punctuation before", "hey @bot.", "@bot", 4},
		{"inside word", "robot", "bot", -1},
		{"inside handle", "@botanic", "@bot", -1},
		{"underscore boundary", "hello bot_name", "bot", -1},
		{"multi word", "hello PicoClaw Bot.", "PicoClaw Bot", 6},
		{"multi word inside larger token", "hello SuperPicoClaw Bot", "PicoClaw Bot", -1},
		{"unicode byte index", "å bot", "bot", 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := TextMentionIndex(tt.content, tt.token); got != tt.want {
				t.Fatalf("TextMentionIndex(%q, %q) = %d, want %d", tt.content, tt.token, got, tt.want)
			}
			if got := ContainsTextMention(tt.content, tt.token); got != (tt.want >= 0) {
				t.Fatalf("ContainsTextMention(%q, %q) = %v, want %v", tt.content, tt.token, got, tt.want >= 0)
			}
		})
	}
}

func TestStripTextMention(t *testing.T) {
	tests := []struct {
		name    string
		content string
		token   string
		want    string
	}{
		{"prefix", "@bot hello", "@bot", "hello"},
		{"suffix", "hello @bot", "@bot", "hello"},
		{"multiple", "@bot hello @bot", "@bot", "hello"},
		{"inside word not stripped", "@botanic @bot", "@bot", "@botanic"},
		{"case insensitive", "@BOT hello", "@bot", "hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := StripTextMention(tt.content, tt.token); got != tt.want {
				t.Fatalf("StripTextMention(%q, %q) = %q, want %q", tt.content, tt.token, got, tt.want)
			}
		})
	}
}
