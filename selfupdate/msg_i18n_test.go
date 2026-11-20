package selfupdate

// 이 포크는 사용자 대면 출력을 선택한 언어로 강제한다(README "Output language").
// selfupdate 의 오류·진행 문구는 update-card 가 그대로 렌더하므로, 번역 표를 거치지
// 않으면 영어 UI 에 한국어가 섞여 나온다(실제로 그렇게 나왔다). 아래 테스트는
// (1) 모든 표 항목이 실제로 번역되고, (2) 포맷 동사가 원문과 순서까지 같은지를 고정한다.
// (2) 는 중요하다: trMsg 를 먼저 거친 뒤 fmt 가 채우므로, 동사가 어긋나면 응답이 깨진다.

import (
	"errors"
	"regexp"
	"strings"
	"testing"

	"github.com/Autumn-27/artex/config"
)

var formatVerb = regexp.MustCompile(`%[-+# 0-9.]*[a-zA-Z%]`)

func withLang(t *testing.T, lang string) {
	t.Helper()
	prev := config.CurrentLanguage
	config.CurrentLanguage = func() string { return lang }
	t.Cleanup(func() { config.CurrentLanguage = prev })
}

// TestUpdateCatalogFormatVerbsMatch pins that every translation keeps the Korean
// template's format verbs in the same order, so errf/msgf fill correctly.
func TestUpdateCatalogFormatVerbsMatch(t *testing.T) {
	for ko, tr := range updateMsgCatalog {
		want := strings.Join(formatVerb.FindAllString(ko, -1), " ")
		for lang, v := range tr {
			if got := strings.Join(formatVerb.FindAllString(v, -1), " "); got != want {
				t.Errorf("updateMsgCatalog[%q][%s] verbs = %q, want %q", ko, lang, got, want)
			}
		}
	}
}

// TestUpdateCatalogTranslatesEveryEntry pins that every catalog entry has all four
// translations and that none of them falls back to the Korean original.
func TestUpdateCatalogTranslatesEveryEntry(t *testing.T) {
	if len(updateMsgCatalog) == 0 {
		t.Fatal("updateMsgCatalog is empty; the scan is broken")
	}
	for ko, tr := range updateMsgCatalog {
		for _, lang := range []string{"en", "zh", "es", "ja"} {
			v, ok := tr[lang]
			if !ok || strings.TrimSpace(v) == "" {
				t.Errorf("updateMsgCatalog[%q] has no %s translation", ko, lang)
				continue
			}
			if v == ko {
				t.Errorf("updateMsgCatalog[%q][%s] is identical to the Korean original", ko, lang)
			}
		}
	}
}

// TestUpdateErrorsAreTranslated pins the real error paths: with English active, an
// error built by errf/errs must not contain Hangul. This is the exact regression
// the settings screen showed ("Can't connect to GitHub: 저장소 ...").
func TestUpdateErrorsAreTranslated(t *testing.T) {
	withLang(t, "en")

	errsToCheck := []error{
		errs("저장소 %s 에 아직 정식 릴리스가 없습니다"),
		errf("GitHub 응답 코드 %d", 500),
		errf("HTTPS 가 아닌 주소를 거부합니다: %s", "http://x"),
	}
	// 저장소 메시지는 포맷 동사를 채워야 하므로 별도로 확인한다.
	if got := errf("저장소 %s 에 아직 정식 릴리스가 없습니다", Repo).Error(); hasHangul(got) || !strings.Contains(got, Repo) {
		t.Errorf("repository error not translated or lost its argument: %q", got)
	}

	for _, err := range errsToCheck {
		if hasHangul(err.Error()) {
			t.Errorf("English output still contains Hangul: %q", err.Error())
		}
	}
}

// TestUpdateProgressMessagesTranslated pins that the progress strings shown in the
// update card are translated too, including the formatted ones.
func TestUpdateProgressMessagesTranslated(t *testing.T) {
	withLang(t, "en")
	for _, s := range []string{
		msgf("%s 내려받는 중(%s)…", "pkg.zip", "12 MB"),
		msgf("내려받는 중 %s / %s", "1 MB", "12 MB"),
		trMsg("체크섬 목록을 가져오는 중…"),
		trMsg("SHA256 검증 중…"),
		trMsg("압축을 풀고 스모크 테스트 중…"),
		trMsg("새 버전이 준비되었습니다. 재시작 중…"),
	} {
		if hasHangul(s) {
			t.Errorf("English progress message still contains Hangul: %q", s)
		}
	}
}

// TestUpdateCatalogWrappedErrorKeepsCause pins that %w still wraps: errf must not
// break errors.Is/As, which the rollback path relies on for os.ErrNotExist.
func TestUpdateCatalogWrappedErrorKeepsCause(t *testing.T) {
	withLang(t, "en")
	err := errf("체크섬 읽기 실패: %w", errors.New("boom"))
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("wrapped cause lost: %q", err.Error())
	}
	if hasHangul(err.Error()) {
		t.Errorf("English wrapped error contains Hangul: %q", err.Error())
	}
}
