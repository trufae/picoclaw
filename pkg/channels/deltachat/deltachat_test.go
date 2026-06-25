package deltachat

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sipeed/picoclaw/pkg/bus"
	"github.com/sipeed/picoclaw/pkg/config"
	"github.com/sipeed/picoclaw/pkg/media"
)

func TestNewDeltaChatChannel(t *testing.T) {
	msgBus := bus.NewMessageBus()

	// A fake rpc server so resolveServerPath succeeds regardless of host setup.
	fakeServer := filepath.Join(t.TempDir(), "deltachat-rpc-server")
	if err := os.WriteFile(fakeServer, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	t.Run("missing email", func(t *testing.T) {
		bc := &config.Channel{Type: config.ChannelDeltaChat, Enabled: true}
		cfg := &config.DeltaChatSettings{Password: *config.NewSecureString("pw"), RPCServerPath: fakeServer}
		if _, err := NewDeltaChatChannel(bc, cfg, msgBus); err == nil {
			t.Error("expected error for missing email")
		}
	})

	t.Run("missing password", func(t *testing.T) {
		bc := &config.Channel{Type: config.ChannelDeltaChat, Enabled: true}
		cfg := &config.DeltaChatSettings{Email: "bot@example.org", RPCServerPath: fakeServer}
		if _, err := NewDeltaChatChannel(bc, cfg, msgBus); err == nil {
			t.Error("expected error for missing password")
		}
	})

	t.Run("missing rpc server", func(t *testing.T) {
		bc := &config.Channel{Type: config.ChannelDeltaChat, Enabled: true}
		cfg := &config.DeltaChatSettings{
			Email:         "bot@example.org",
			Password:      *config.NewSecureString("pw"),
			RPCServerPath: filepath.Join(t.TempDir(), "does-not-exist"),
		}
		if _, err := NewDeltaChatChannel(bc, cfg, msgBus); err == nil {
			t.Error("expected error for missing rpc server path")
		}
	})

	t.Run("valid config", func(t *testing.T) {
		bc := &config.Channel{Type: config.ChannelDeltaChat, Enabled: true}
		cfg := &config.DeltaChatSettings{
			Email:         "bot@example.org",
			Password:      *config.NewSecureString("pw"),
			RPCServerPath: fakeServer,
			DataDir:       t.TempDir(),
		}
		ch, err := NewDeltaChatChannel(bc, cfg, msgBus)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ch.Name() != config.ChannelDeltaChat {
			t.Errorf("Name() = %q, want %q", ch.Name(), config.ChannelDeltaChat)
		}
		if ch.IsRunning() {
			t.Error("new channel should not be running")
		}
	})
}

func TestMentionsBot(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		displayName string
		email       string
		want        bool
	}{
		{"display name", "hey PicoBot can you help", "PicoBot", "bot@example.org", true},
		{"case insensitive name", "hey picobot", "PicoBot", "bot@example.org", true},
		{"email local part", "@bot please summarize", "", "bot@example.org", true},
		{"no mention", "just chatting here", "PicoBot", "bot@example.org", false},
		{"local part without @", "the robot is cool", "", "bot@example.org", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := mentionsBot(tt.content, tt.displayName, tt.email); got != tt.want {
				t.Errorf("mentionsBot(%q, %q, %q) = %v, want %v", tt.content, tt.displayName, tt.email, got, tt.want)
			}
		})
	}
}

func TestExpandHome(t *testing.T) {
	home, _ := os.UserHomeDir()
	tests := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"/abs/path", "/abs/path"},
		{"~", home},
		{"~/sub", filepath.Join(home, "sub")},
		{"relative", "relative"},
	}
	for _, tt := range tests {
		if got := expandHome(tt.in); got != tt.want {
			t.Errorf("expandHome(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestResolveDataDir(t *testing.T) {
	if got := resolveDataDir("/explicit/dir", "x"); got != "/explicit/dir" {
		t.Errorf("explicit data dir = %q, want /explicit/dir", got)
	}
	home, _ := os.UserHomeDir()
	want := filepath.Join(home, ".picoclaw", "deltachat", "mychan")
	if got := resolveDataDir("", "mychan"); got != want {
		t.Errorf("default data dir = %q, want %q", got, want)
	}
}

func TestHandleMessageMarksSeenOnlyAfterDispatch(t *testing.T) {
	tests := []struct {
		name        string
		chatType    string
		mentionOnly bool
		closeBus    bool
		wantSeen    bool
	}{
		{name: "successful dispatch", chatType: chatTypeSingle, wantSeen: true},
		{name: "ignored group trigger", chatType: "Group", mentionOnly: true},
		{name: "failed local publish", chatType: chatTypeSingle, closeBus: true},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messageID := int64(42 + i)
			chat := dcChat{ID: 99, Name: "chat", ChatType: tt.chatType}
			msgBus := bus.NewMessageBus()
			if tt.closeBus {
				msgBus.Close()
			} else {
				defer msgBus.Close()
			}

			ch := newTestChannelWithBus(t, msgBus, func(bc *config.Channel) {
				bc.GroupTrigger.MentionOnly = tt.mentionOnly
			})
			ch.ctx = context.Background()
			ch.accountID = 7
			ch.selfAddr = "bot@example.org"

			markSeen := make(chan struct{}, 1)
			rpc, cleanup := newMockRPC(t, func(req rpcRequest) string {
				switch req.Method {
				case "get_message":
					return rpcResult(req, dcMessage{
						ID:     messageID,
						ChatID: chat.ID,
						Text:   "hello",
						Sender: &dcContact{Address: "alice@example.org", DisplayName: "Alice"},
					})
				case "get_full_chat_by_id":
					return rpcResult(req, chat)
				case "markseen_msgs":
					markSeen <- struct{}{}
					return rpcResult(req, nil)
				default:
					return `{"jsonrpc":"2.0","id":` + itoa(req.ID) + `,"error":{"code":-32601,"message":"unexpected method"}}`
				}
			})
			defer cleanup()
			ch.rpc = rpc

			ch.handleMessage(messageID)

			gotSeen := false
			select {
			case <-markSeen:
				gotSeen = true
			default:
			}
			if gotSeen != tt.wantSeen {
				t.Fatalf("markseen called = %v, want %v", gotSeen, tt.wantSeen)
			}
		})
	}
}

func TestDeltaChatSettingsDecode(t *testing.T) {
	raw := []byte(`{
		"enabled": true,
		"type": "deltachat",
		"allow_from": ["alice@example.org"],
		"settings": {
			"email": "bot@example.org",
			"display_name": "PicoBot",
			"imap_port": 993
		}
	}`)
	var bc config.Channel
	if err := json.Unmarshal(raw, &bc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	bc.Type = config.ChannelDeltaChat
	decoded, err := bc.GetDecoded()
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	cfg, ok := decoded.(*config.DeltaChatSettings)
	if !ok {
		t.Fatalf("decoded type = %T, want *config.DeltaChatSettings", decoded)
	}
	if cfg.Email != "bot@example.org" {
		t.Errorf("email = %q, want bot@example.org", cfg.Email)
	}
	if cfg.DisplayName != "PicoBot" {
		t.Errorf("display_name = %q, want PicoBot", cfg.DisplayName)
	}
	if cfg.IMAPPort != 993 {
		t.Errorf("imap_port = %d, want 993", cfg.IMAPPort)
	}
}

// TestRPCClientRoundTrip drives the JSON-RPC client against an in-process mock
// server over pipes, verifying id correlation and error propagation.
func TestRPCClientRoundTrip(t *testing.T) {
	reqR, reqW := io.Pipe()   // client -> server
	respR, respW := io.Pipe() // server -> client

	c := &rpcClient{
		stdin:   reqW,
		stdout:  respR,
		pending: make(map[uint64]chan rpcResponse),
	}
	go c.readLoop()

	// Mock server: echo method "ping" -> "pong", anything else -> error.
	go func() {
		scanner := bufio.NewScanner(reqR)
		for scanner.Scan() {
			var req rpcRequest
			if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
				continue
			}
			var resp string
			if req.Method == "ping" {
				resp = `{"jsonrpc":"2.0","id":` + itoa(req.ID) + `,"result":"pong"}`
			} else {
				resp = `{"jsonrpc":"2.0","id":` + itoa(req.ID) + `,"error":{"code":-1,"message":"boom"}}`
			}
			_, _ = respW.Write([]byte(resp + "\n"))
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	raw, err := c.call(ctx, "ping")
	if err != nil {
		t.Fatalf("ping call: %v", err)
	}
	var result string
	if err := json.Unmarshal(raw, &result); err != nil || result != "pong" {
		t.Fatalf("ping result = %q (err %v), want pong", result, err)
	}

	if _, err := c.call(ctx, "explode"); err == nil {
		t.Fatal("expected error from explode call")
	}
}

func itoa(n uint64) string {
	b, _ := json.Marshal(n)
	return string(b)
}

// newTestChannel builds a DeltaChatChannel with a valid config (backed by a fake
// rpc-server binary) but without starting any IO, for unit-testing methods in
// isolation.
func newTestChannel(t *testing.T) *DeltaChatChannel {
	return newTestChannelWithBus(t, bus.NewMessageBus(), nil)
}

func newTestChannelWithBus(t *testing.T, msgBus *bus.MessageBus, configure func(*config.Channel)) *DeltaChatChannel {
	t.Helper()
	fakeServer := filepath.Join(t.TempDir(), "deltachat-rpc-server")
	if err := os.WriteFile(fakeServer, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	bc := &config.Channel{Type: config.ChannelDeltaChat, Enabled: true}
	if configure != nil {
		configure(bc)
	}
	cfg := &config.DeltaChatSettings{
		Email:         "bot@example.org",
		Password:      *config.NewSecureString("pw"),
		RPCServerPath: fakeServer,
		DataDir:       t.TempDir(),
	}
	ch, err := NewDeltaChatChannel(bc, cfg, msgBus)
	if err != nil {
		t.Fatalf("new channel: %v", err)
	}
	return ch
}

// newMockRPC wires an rpcClient to an in-process server that replies to every
// request with handler(req), so methods that call the rpc can be tested without
// a real deltachat-rpc-server.
func newMockRPC(t *testing.T, handler func(req rpcRequest) string) (*rpcClient, func()) {
	t.Helper()
	reqR, reqW := io.Pipe()
	respR, respW := io.Pipe()
	c := &rpcClient{
		stdin:   reqW,
		stdout:  respR,
		pending: make(map[uint64]chan rpcResponse),
	}
	go c.readLoop()
	go func() {
		scanner := bufio.NewScanner(reqR)
		for scanner.Scan() {
			var req rpcRequest
			if err := json.Unmarshal(scanner.Bytes(), &req); err != nil {
				continue
			}
			_, _ = respW.Write([]byte(handler(req) + "\n"))
		}
	}()
	return c, func() { _ = reqW.Close(); _ = respW.Close() }
}

func rpcResult(req rpcRequest, result any) string {
	raw, _ := json.Marshal(result)
	return `{"jsonrpc":"2.0","id":` + itoa(req.ID) + `,"result":` + string(raw) + `}`
}

// TestMessageDataJSON pins the camelCase keys and omitempty behavior expected by
// Delta Chat's send_msg MessageData parameter.
func TestMessageDataJSON(t *testing.T) {
	raw, err := json.Marshal(dcMessageData{File: "/tmp/x.png"})
	if err != nil {
		t.Fatal(err)
	}
	if got := string(raw); got != `{"file":"/tmp/x.png"}` {
		t.Errorf("json = %s, want only the file field", got)
	}

	raw, _ = json.Marshal(dcMessageData{Text: "hi", File: "/f", Filename: "f.bin"})
	if got := string(raw); got != `{"text":"hi","file":"/f","filename":"f.bin"}` {
		t.Errorf("json = %s, want text/file/filename in camelCase", got)
	}
}

// TestRegisterInboundFile checks that an inbound attachment is copied out of
// Delta Chat's account directory into the tool-readable media temp dir and
// registered with delete-on-cleanup, and that the absence of a store yields an
// empty ref for the annotation fallback.
func TestRegisterInboundFile(t *testing.T) {
	ch := newTestChannel(t)

	tmp := filepath.Join(t.TempDir(), "doc.pdf")
	if err := os.WriteFile(tmp, []byte("%PDF-1.4"), 0o644); err != nil {
		t.Fatal(err)
	}
	msg := &dcMessage{File: tmp, FileName: "doc.pdf", FileMime: "application/pdf"}

	if ref := ch.registerInboundFile("scope", msg); ref != "" {
		t.Errorf("ref without media store = %q, want empty", ref)
	}

	store := media.NewFileMediaStore()
	ch.SetMediaStore(store)
	ref := ch.registerInboundFile("scope", msg)
	if !strings.HasPrefix(ref, "media://") {
		t.Fatalf("ref = %q, want a media:// ref", ref)
	}
	path, meta, err := store.ResolveWithMeta(ref)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(path) })

	// The registered path must be a copy in the media temp dir (tool-readable),
	// not the original blob path, and must have the same contents.
	if path == tmp {
		t.Errorf("path = %q, want a copy in the media temp dir, not the blob path", path)
	}
	if !strings.HasPrefix(filepath.Clean(path), filepath.Clean(media.TempDir())) {
		t.Errorf("path = %q, want it under media temp dir %q", path, media.TempDir())
	}
	if data, rerr := os.ReadFile(path); rerr != nil || string(data) != "%PDF-1.4" {
		t.Errorf("copied file contents = %q (err %v), want %q", string(data), rerr, "%PDF-1.4")
	}
	if meta.ContentType != "application/pdf" {
		t.Errorf("content type = %q, want application/pdf", meta.ContentType)
	}
	if meta.CleanupPolicy != media.CleanupPolicyDeleteOnCleanup {
		t.Errorf("cleanup policy = %q, want delete_on_cleanup", meta.CleanupPolicy)
	}
}

// TestSendMedia verifies SendMedia resolves a media ref to a local path and
// drives send_msg with the expected MessageData, returning the new message id.
func TestSendMedia(t *testing.T) {
	ch := newTestChannel(t)

	tmp := filepath.Join(t.TempDir(), "photo.png")
	if err := os.WriteFile(tmp, []byte("\x89PNGfake"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := media.NewFileMediaStore()
	ch.SetMediaStore(store)
	ref, err := store.Store(tmp, media.MediaMeta{Filename: "photo.png"}, "scope")
	if err != nil {
		t.Fatal(err)
	}

	captured := make(chan rpcRequest, 1)
	rpc, cleanup := newMockRPC(t, func(req rpcRequest) string {
		captured <- req
		return `{"jsonrpc":"2.0","id":` + itoa(req.ID) + `,"result":4242}`
	})
	defer cleanup()
	ch.rpc = rpc
	ch.accountID = 7
	ch.SetRunning(true)

	ids, err := ch.SendMedia(context.Background(), bus.OutboundMediaMessage{
		ChatID: "99",
		Parts: []bus.MediaPart{{
			Type:     "image",
			Ref:      ref,
			Caption:  "hello",
			Filename: "photo.png",
		}},
	})
	if err != nil {
		t.Fatalf("SendMedia: %v", err)
	}
	if len(ids) != 1 || ids[0] != "4242" {
		t.Fatalf("ids = %v, want [4242]", ids)
	}

	select {
	case req := <-captured:
		if req.Method != "send_msg" {
			t.Errorf("method = %q, want send_msg", req.Method)
		}
		if len(req.Params) != 3 {
			t.Fatalf("params = %v, want [accountID, chatID, data]", req.Params)
		}
		if got, _ := req.Params[0].(float64); got != 7 {
			t.Errorf("account id = %v, want 7", req.Params[0])
		}
		if got, _ := req.Params[1].(float64); got != 99 {
			t.Errorf("chat id = %v, want 99", req.Params[1])
		}
		data, ok := req.Params[2].(map[string]any)
		if !ok {
			t.Fatalf("data param = %T, want object", req.Params[2])
		}
		if data["file"] != tmp {
			t.Errorf("file = %v, want %s", data["file"], tmp)
		}
		if data["text"] != "hello" {
			t.Errorf("text = %v, want hello", data["text"])
		}
		if data["filename"] != "photo.png" {
			t.Errorf("filename = %v, want photo.png", data["filename"])
		}
		if _, present := data["viewtype"]; present {
			t.Errorf("viewtype should be omitted (Delta Chat infers it), got %v", data["viewtype"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("mock server never received the request")
	}
}

// TestSendMediaNoStore ensures SendMedia fails cleanly without a media store.
func TestSendMediaNoStore(t *testing.T) {
	ch := newTestChannel(t)
	ch.SetRunning(true)
	if _, err := ch.SendMedia(context.Background(), bus.OutboundMediaMessage{ChatID: "1"}); err == nil {
		t.Error("expected error when no media store is configured")
	}
}

// TestSendMediaVoice verifies that a send_tts-sourced audio part is delivered
// with viewtype "Voice" so Delta Chat renders it as a voice bubble.
func TestSendMediaVoice(t *testing.T) {
	ch := newTestChannel(t)

	tmp := filepath.Join(t.TempDir(), "tts-123.ogg")
	if err := os.WriteFile(tmp, []byte("OggSfake"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := media.NewFileMediaStore()
	ch.SetMediaStore(store)
	ref, err := store.Store(tmp, media.MediaMeta{
		Filename:    "tts-123.ogg",
		ContentType: "audio/ogg",
		Source:      "tool:send_tts",
	}, "scope")
	if err != nil {
		t.Fatal(err)
	}

	captured := make(chan rpcRequest, 1)
	rpc, cleanup := newMockRPC(t, func(req rpcRequest) string {
		captured <- req
		return `{"jsonrpc":"2.0","id":` + itoa(req.ID) + `,"result":7}`
	})
	defer cleanup()
	ch.rpc = rpc
	ch.accountID = 1
	ch.SetRunning(true)

	if _, err := ch.SendMedia(context.Background(), bus.OutboundMediaMessage{
		ChatID: "5",
		Parts:  []bus.MediaPart{{Type: "audio", Ref: ref, ContentType: "audio/ogg"}},
	}); err != nil {
		t.Fatalf("SendMedia: %v", err)
	}

	select {
	case req := <-captured:
		data, ok := req.Params[2].(map[string]any)
		if !ok {
			t.Fatalf("data param = %T, want object", req.Params[2])
		}
		if data["viewtype"] != "Voice" {
			t.Errorf("viewtype = %v, want Voice", data["viewtype"])
		}
	case <-time.After(2 * time.Second):
		t.Fatal("mock server never received the request")
	}
}

// TestDeltaChatViewtype pins the rule that only voice audio is forced to a view
// type; everything else is left to Delta Chat's own detection.
func TestDeltaChatViewtype(t *testing.T) {
	tests := []struct {
		name string
		part bus.MediaPart
		meta media.MediaMeta
		want string
	}{
		{
			"tts audio",
			bus.MediaPart{Type: "audio"},
			media.MediaMeta{Source: "tool:send_tts", ContentType: "audio/ogg"},
			"Voice",
		},
		{"voice filename", bus.MediaPart{Type: "audio", Filename: "my-voice.mp3"}, media.MediaMeta{}, "Voice"},
		{
			"plain audio",
			bus.MediaPart{Type: "audio", Filename: "song.mp3"},
			media.MediaMeta{ContentType: "audio/mpeg"},
			"",
		},
		{"image", bus.MediaPart{Type: "image", Filename: "photo.png"}, media.MediaMeta{ContentType: "image/png"}, ""},
		{"file", bus.MediaPart{Type: "file", Filename: "doc.pdf"}, media.MediaMeta{}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deltaChatViewtype(tt.part, tt.meta); got != tt.want {
				t.Errorf("deltaChatViewtype() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestVoiceCapabilities checks that Delta Chat advertises ASR and TTS so the
// gateway's startup capability log is accurate.
func TestVoiceCapabilities(t *testing.T) {
	ch := newTestChannel(t)
	caps := ch.VoiceCapabilities()
	if !caps.ASR || !caps.TTS {
		t.Errorf("VoiceCapabilities() = %+v, want both ASR and TTS true", caps)
	}
}
