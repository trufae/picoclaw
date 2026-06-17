> Back to [README](../../../README.md)

# Delta Chat Channel

PicoClaw can run as a Delta Chat bot by launching a local
`deltachat-rpc-server` process and talking to it over JSON-RPC. The RPC server
handles the email account, IMAP/SMTP connection, message store, and encryption
keys.

## Install

Install the Delta Chat RPC server, then set `rpc_server_path` to the exact
binary path. PicoClaw does not search `PATH`.

```bash
pip install deltachat-rpc-server
which deltachat-rpc-server
```

Prebuilt binaries are also available from the
[Delta Chat core releases](https://github.com/deltachat/deltachat-core-rust/releases).

## Configure

Add a channel entry under `channel_list`:

```json
{
  "channel_list": {
    "deltachat": {
      "enabled": true,
      "type": "deltachat",
      "allow_from": ["friend@example.org"],
      "group_trigger": {
        "mention_only": true
      },
      "settings": {
        "email": "bot@example.org",
        "password": "your-app-password",
        "display_name": "PicoClaw Bot",
        "rpc_server_path": "/home/me/.venv/bin/deltachat-rpc-server"
      }
    }
  }
}
```

`password` is a secure field. On first config load it is moved to
`~/.picoclaw/.security.yml`. You can also set it with
`PICOCLAW_CHANNELS_DELTACHAT_PASSWORD`. For providers with two-factor
authentication, use an app-specific password.

| Field | Required | Description |
|-------|----------|-------------|
| `email` | Yes | Bot mailbox address |
| `password` | Yes | Mailbox or app-specific password, stored in `.security.yml` |
| `rpc_server_path` | Yes | Path to `deltachat-rpc-server`; `~` is expanded |
| `display_name` | No | Name shown to contacts and used for group mention detection |
| `data_dir` | No | Account database directory. Default: `~/.picoclaw/deltachat/<channel-name>` |
| `invite_link` | No | Delta Chat invite link to join on startup |
| `imap_server`, `imap_port` | No | Manual IMAP override |
| `smtp_server`, `smtp_port` | No | Manual SMTP override |

Standard channel fields such as `allow_from`, `group_trigger`, and
`reasoning_channel_id` also apply.

## First Run

With a new `data_dir`, PicoClaw configures the account and validates the
mailbox credentials. After that, the account is reused from the local data
directory.

Delta Chat requires peers to learn the bot's encryption key before messaging
it. On startup PicoClaw prints the bot invite link and QR code. Add the bot from
Delta Chat with that invite, not by typing the bare email address.

## Behavior

- Direct chats always respond after `allow_from` passes.
- Group chats follow `group_trigger`; without one, every group message is
  handled.
- Messages from the bot itself, device chats, and info/system messages are
  ignored.
- Accepted inbound messages are marked seen after the allow-list check.
- Incoming attachments (images, audio, video, documents) are registered with
  the media store and handed to the agent, so it can view images or operate on
  the files directly. If no media store is available, the path is appended
  inline as `[attachment: /path]` instead.
- Outbound attachments are supported: when the agent emits media, each file is
  sent as a Delta Chat message (with the caption as text). Delta Chat infers the
  view type from the file, so images, GIFs, and videos render natively.
- Voice is supported in both directions when voice providers are configured:
  incoming voice notes are transcribed by the agent's ASR and the transcript is
  passed to the model; the agent can reply with synthesized speech, which is
  delivered as a native Delta Chat voice message (`send_tts`). This requires an
  ASR and/or TTS provider under `voice` — it is not Delta Chat-specific config.

## Troubleshooting

| Symptom | Fix |
|---------|-----|
| `rpc_server_path is required` or `not found` | Install the RPC server and set an absolute path |
| `configure (check email/password/server)` | Check credentials, app password requirements, or IMAP/SMTP overrides |
| Bot does not answer in a group | Check `group_trigger`; mention `display_name` or use a configured prefix |
| Bot ignores a sender | Add the sender email to `allow_from`, or use `["*"]` for open access |
| Sender cannot message the bot | Re-add the bot with the startup QR/invite so Delta Chat can establish encryption |
