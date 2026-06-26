package line

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/line/line-bot-sdk-go/v8/linebot/webhook"

	"github.com/sipeed/picoclaw/pkg/config"
)

func TestWebhookRejectsOversizedBody(t *testing.T) {
	ch := &LINEChannel{config: &config.LINESettings{}}

	oversized := bytes.Repeat([]byte("A"), maxWebhookBodySize+1)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(oversized))
	rec := httptest.NewRecorder()

	ch.webhookHandler(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected status %d, got %d", http.StatusRequestEntityTooLarge, rec.Code)
	}
}

func TestWebhookAcceptsMaxBodySize(t *testing.T) {
	ch := &LINEChannel{config: &config.LINESettings{}}

	body := bytes.Repeat([]byte("A"), maxWebhookBodySize)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(body))
	rec := httptest.NewRecorder()

	ch.webhookHandler(rec, req)

	// Missing signature should be rejected, but the body size should not trigger 413.
	if rec.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, rec.Code)
	}
}

func TestWebhookRejectsOversizedBodyBeforeSignatureCheck(t *testing.T) {
	ch := &LINEChannel{config: &config.LINESettings{}}

	oversized := bytes.Repeat([]byte("A"), maxWebhookBodySize+1)
	req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(oversized))
	req.Header.Set("X-Line-Signature", "invalidsignature")
	rec := httptest.NewRecorder()

	ch.webhookHandler(rec, req)

	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("expected status %d, got %d", http.StatusRequestEntityTooLarge, rec.Code)
	}
}

func TestWebhookRejectsNonPostMethod(t *testing.T) {
	ch := &LINEChannel{config: &config.LINESettings{}}

	req := httptest.NewRequest(http.MethodGet, "/webhook", nil)
	rec := httptest.NewRecorder()

	ch.webhookHandler(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, rec.Code)
	}
}

func TestWebhookRejectsInvalidSignature(t *testing.T) {
	ch := &LINEChannel{
		config: &config.LINESettings{},
	}

	body := `{"events":[]}`
	req := httptest.NewRequest(http.MethodPost, "/webhook", strings.NewReader(body))
	req.Header.Set("X-Line-Signature", "invalidsignature")
	rec := httptest.NewRecorder()

	ch.webhookHandler(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("expected status %d, got %d", http.StatusForbidden, rec.Code)
	}
}

func TestIsBotMentionedUsesTextMentionBoundaries(t *testing.T) {
	ch := &LINEChannel{botDisplayName: "AI"}

	tests := []struct {
		name string
		msg  webhook.TextMessageContent
		want bool
	}{
		{
			name: "fallback exact display name",
			msg:  webhook.TextMessageContent{Text: "@AI help"},
			want: true,
		},
		{
			name: "fallback inside word",
			msg:  webhook.TextMessageContent{Text: "please email me later"},
			want: false,
		},
		{
			name: "metadata mention exact display name",
			msg: webhook.TextMessageContent{
				Text: "@AI help",
				Mention: &webhook.Mention{Mentionees: []webhook.MentioneeInterface{
					&webhook.UserMentionee{Mentionee: webhook.Mentionee{Index: 0, Length: 3}},
				}},
			},
			want: true,
		},
		{
			name: "metadata mention inside word",
			msg: webhook.TextMessageContent{
				Text: "@MAIL help",
				Mention: &webhook.Mention{Mentionees: []webhook.MentioneeInterface{
					&webhook.UserMentionee{Mentionee: webhook.Mentionee{Index: 0, Length: 5}},
				}},
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ch.isBotMentioned(tt.msg); got != tt.want {
				t.Fatalf("isBotMentioned() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStripBotMentionUsesTextMentionBoundaries(t *testing.T) {
	ch := &LINEChannel{botDisplayName: "Bot"}

	got := ch.stripBotMention("@Botanic @Bot hello", webhook.TextMessageContent{Text: "@Botanic @Bot hello"})
	if got != "@Botanic hello" {
		t.Fatalf("stripBotMention() = %q, want %q", got, "@Botanic hello")
	}
}
