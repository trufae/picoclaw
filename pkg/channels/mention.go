package channels

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// TextMentionIndex returns the byte offset where token appears as a standalone
// plain-text mention in content, or -1 if not found.
func TextMentionIndex(content, token string) int {
	start, _ := textMentionRange(content, token)
	return start
}

// ContainsTextMention reports whether token appears as a standalone plain-text
// mention in content.
func ContainsTextMention(content, token string) bool {
	return TextMentionIndex(content, token) >= 0
}

// StripTextMention removes standalone plain-text occurrences of token from
// content.
func StripTextMention(content, token string) string {
	for {
		start, end := textMentionRange(content, token)
		if start < 0 {
			break
		}
		if hasSpaceBefore(content, start) {
			if ok, width := hasSpaceAt(content, end); ok {
				end += width
			}
		}
		content = content[:start] + content[end:]
	}
	return strings.TrimSpace(content)
}

func textMentionRange(content, token string) (int, int) {
	token = strings.TrimSpace(token)
	if token == "" {
		return -1, -1
	}

	contentRunes := []rune(strings.ToLower(content))
	tokenRunes := []rune(strings.ToLower(token))
	if len(tokenRunes) == 0 || len(tokenRunes) > len(contentRunes) {
		return -1, -1
	}

	offsets := runeByteOffsets(content)
	for i := 0; i <= len(contentRunes)-len(tokenRunes); i++ {
		if !sameMentionRunes(contentRunes[i:i+len(tokenRunes)], tokenRunes) {
			continue
		}
		before := i == 0 || !isTextMentionWordRune(contentRunes[i-1])
		afterIdx := i + len(tokenRunes)
		after := afterIdx >= len(contentRunes) || !isTextMentionWordRune(contentRunes[afterIdx])
		if before && after {
			end := len(content)
			if afterIdx < len(offsets) {
				end = offsets[afterIdx]
			}
			return offsets[i], end
		}
	}
	return -1, -1
}

func runeByteOffsets(s string) []int {
	offsets := make([]int, 0, utf8.RuneCountInString(s))
	for idx := range s {
		offsets = append(offsets, idx)
	}
	return offsets
}

func sameMentionRunes(a, b []rune) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func isTextMentionWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

func hasSpaceBefore(s string, idx int) bool {
	if idx <= 0 || idx > len(s) {
		return false
	}
	r, _ := utf8.DecodeLastRuneInString(s[:idx])
	return unicode.IsSpace(r)
}

func hasSpaceAt(s string, idx int) (bool, int) {
	if idx < 0 || idx >= len(s) {
		return false, 0
	}
	r, width := utf8.DecodeRuneInString(s[idx:])
	return unicode.IsSpace(r), width
}
