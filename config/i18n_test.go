package config

import (
	"regexp"
	"strings"
	"testing"
)

// TestZhMsgCatalogComplete pins that every Chinese-keyed entry carries a
// translation for all supported output languages and that each translation
// actually differs from the Chinese source. TrMsg wraps the template before
// fmt.Errorf fills it, so a missing entry would leak Chinese to an English user.
func TestZhMsgCatalogComplete(t *testing.T) {
	t.Cleanup(func() { CurrentLanguage = nil })
	for zh, tr := range zhMsgCatalog {
		if len(tr) == 0 {
			t.Errorf("zhMsgCatalog[%q] has no translations", zh)
		}
		for _, lang := range []string{"en", "ko", "es", "ja"} {
			v, ok := tr[lang]
			if !ok || v == "" {
				t.Errorf("zhMsgCatalog[%q] missing %s translation", zh, lang)
				continue
			}
			if v == zh {
				t.Errorf("zhMsgCatalog[%q][%s] equals the source", zh, lang)
			}
			CurrentLanguage = func() string { return lang }
			if got := TrMsg(zh); got != v {
				t.Errorf("TrMsg(%q) with lang=%s = %q, want %q", zh, lang, got, v)
			}
		}
	}
}

// TestZhMsgCatalogFormatVerbsMatch pins that translated templates keep the source
// template's format verbs in the same order; TrMsg wraps the template before
// fmt.Sprintf/fmt.Errorf fill it, so a mismatch would garble the message.
func TestZhMsgCatalogFormatVerbsMatch(t *testing.T) {
	verbs := regexp.MustCompile(`%[-+# 0-9.]*[a-zA-Z%]`)
	for zh, tr := range zhMsgCatalog {
		want := strings.Join(verbs.FindAllString(zh, -1), " ")
		for lang, v := range tr {
			if got := strings.Join(verbs.FindAllString(v, -1), " "); got != want {
				t.Errorf("zhMsgCatalog[%q][%s] verbs = %q, want %q", zh, lang, got, want)
			}
		}
	}
}

// TestTrMsgReturnsSourceForChinese pins that a user who selected Chinese sees the
// original literal unchanged, and that an unknown key passes through.
func TestTrMsgReturnsSourceForChinese(t *testing.T) {
	t.Cleanup(func() { CurrentLanguage = nil })
	CurrentLanguage = func() string { return "zh" }
	if got := TrMsg("空行"); got != "空行" {
		t.Errorf("TrMsg with zh = %q, want the source", got)
	}
	if got := TrMsg("no such key"); got != "no such key" {
		t.Errorf("TrMsg for unknown key = %q, want the input", got)
	}
}
