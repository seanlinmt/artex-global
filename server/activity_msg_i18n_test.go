package server

import (
	"encoding/json"
	"strings"
	"testing"
	"unicode"

	"github.com/Autumn-27/artex/agent"
	"github.com/Autumn-27/artex/config"
	"github.com/Autumn-27/artex/db"
)

func TestActivityDTOTranslatesTerminalSummary(t *testing.T) {
	t.Cleanup(func() { config.CurrentLanguage = nil })
	config.CurrentLanguage = func() string { return "en" }

	for _, suffix := range []string{"…", "..."} {
		src := "（无文字总结，终态 model_error：模型或 API 调用失败（网络、鉴权、限流、供应商 5xx 等），重试用尽后意图标记为 blocked——传输层故障导致这条意图基本没真正探成；查其执行过程（g" + suffix + "）"
		got := activityDTO(db.Activity{Kind: "result", Summary: src}).Summary
		want := "(no written summary, terminal state model_error: The model or API call failed (network, auth, rate limiting, provider 5xx, etc.); after retries were exhausted the intent is marked blocked — a transport-layer fault means this intent was essentially never actually explored; inspect its execution trace (get_worker_trace) before deciding to re-dispatch it or change approach)"
		if got != want {
			t.Fatalf("suffix %q: got %q, want %q", suffix, got, want)
		}
	}
}

func TestDisplayDetailTranslatesTerminalBlock(t *testing.T) {
	t.Cleanup(func() { config.CurrentLanguage = nil })
	config.CurrentLanguage = func() string { return "en" }

	detail := strings.Join([]string{
		"- **终态**: `model_error` - 模型或 API 调用失败（网络、鉴权、限流、供应商 5xx 等），重试用尽后意图标记为 blocked——传输层故障导致这条意图基本没真正探成；查其执行过程（get_worker_trace）后再决定重派或换法",
		"- **底层错误**: `Post \"https://overwatch.clearpixels.org:4443/v1/chat/completions\": dial tcp: lookup overwatch.clearpixels.org: no such host`",
		"- **已执行**: 18 轮模型回合",
		"- **本次运行耗时**: 15m57s",
		"- **累计 token**: 输入 1265062 / 输出 34166 / 缓存读 1157120 / 缓存写 0",
		"- **中断前最后一个工具**: `Bash`（已正常返回）",
	}, "\n")

	got := agent.DisplayDetail(detail)
	wantLines := []string{
		"- **Terminal state**: `model_error` - The model or API call failed",
		"- **Underlying error**: `Post \"https://overwatch.clearpixels.org:4443/v1/chat/completions\": dial tcp: lookup overwatch.clearpixels.org: no such host`",
		"- **Executed**: 18 model turns",
		"- **This run's duration**: 15m57s",
		"- **Cumulative tokens**: input 1265062 / output 34166 / cache read 1157120 / cache write 0",
		"- **Last tool before interruption**: `Bash` (returned normally)",
	}
	for _, want := range wantLines {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in DisplayDetail output:\n%s", want, got)
		}
	}

	const nonTerminal = "plain bash stdout output\nline 2"
	if got := agent.DisplayDetail(nonTerminal); got != nonTerminal {
		t.Fatalf("non-terminal detail should pass through unchanged, got %q", got)
	}
}

func TestActivityDTOTranslatesTranscriptErrors(t *testing.T) {
	t.Cleanup(func() { config.CurrentLanguage = nil })

	mainErr := transcriptErrorSummary("메인 에이전트", `Post "https://example.com": no such host`)
	chatErr := transcriptErrorSummary("", "timeout")

	config.CurrentLanguage = func() string { return "en" }
	if got, want := activityDTO(db.Activity{Kind: "text", IsError: true, Summary: mainErr}).Summary, `(Main agent error: Post "https://example.com": no such host)`; got != want {
		t.Fatalf("en main error = %q, want %q", got, want)
	}
	if got, want := activityDTO(db.Activity{Kind: "text", IsError: true, Summary: chatErr}).Summary, "(error: timeout)"; got != want {
		t.Fatalf("en chat error = %q, want %q", got, want)
	}

	config.CurrentLanguage = func() string { return "ko" }
	if got := activityDTO(db.Activity{Kind: "text", IsError: true, Summary: mainErr}).Summary; got != mainErr {
		t.Fatalf("ko main error = %q, want %q", got, mainErr)
	}
}

func TestTaskNodeDTOTranslatesSeedIntent(t *testing.T) {
	t.Cleanup(func() { config.CurrentLanguage = nil })

	node := &db.Node{
		ID:      223,
		Kind:    "intent",
		Payload: json.RawMessage(`{"summary":"完成任务目标：Run a vuln check（任务：desc）"}`),
		State:   "done",
		Origin:  "seed",
	}

	config.CurrentLanguage = func() string { return "en" }
	enDTO := taskNodeDTO(node)
	var enPayload map[string]string
	if err := json.Unmarshal([]byte(enDTO.Payload), &enPayload); err != nil {
		t.Fatalf("unmarshal en payload: %v", err)
	}
	if want := "Complete task goal: Run a vuln check (Task: desc)"; enPayload["summary"] != want {
		t.Fatalf("en seed intent summary = %q, want %q", enPayload["summary"], want)
	}

	config.CurrentLanguage = func() string { return "zh" }
	zhDTO := taskNodeDTO(node)
	var zhPayload map[string]string
	if err := json.Unmarshal([]byte(zhDTO.Payload), &zhPayload); err != nil {
		t.Fatalf("unmarshal zh payload: %v", err)
	}
	if want := "完成任务目标：Run a vuln check（任务：desc）"; zhPayload["summary"] != want {
		t.Fatalf("zh seed intent summary = %q, want %q", zhPayload["summary"], want)
	}
}

func containsHan(s string) bool {
	for _, r := range s {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

func TestDisplayDetailTranslatesAbortedStreamingHeaderAndCause(t *testing.T) {
	t.Cleanup(func() { config.CurrentLanguage = nil })
	config.CurrentLanguage = func() string { return "en" }

	detail := strings.Join([]string{
		"（运行被中断：用户停止了本轮对话；停在模型输出阶段，已运行 1 轮 / 18.6s，未完成）",
		"",
		"- **终态**: `aborted_streaming` - 运行在模型输出流式生成阶段被取消",
		"- **中断原因** (`chat_stopped_by_user`): 用户点击了停止，主动中止本轮主 Agent 或会话 Agent 运行。已经产生的活动记录会保留，可以继续发送下一条消息",
		"- **底层错误**: `context canceled`",
		"- **已执行**: 1 轮模型回合",
		"- **本次运行耗时**: 18.6s",
		"- **中断时正在执行的工具**: `Compress`（已运行 0s，**未返回结果**）",
	}, "\n")

	got := agent.DisplayDetail(detail)
	if containsHan(got) {
		t.Fatalf("translated detail still contains Han characters:\n%s", got)
	}
	wantLines := []string{
		"(run interrupted: user stopped this conversation turn; stopped in the model-output stage; ran for 1 turns / 18.6s, incomplete)",
		"- **Terminal state**: `aborted_streaming` - The run was cancelled while the model was streaming its output",
		"- **Interruption cause** (`chat_stopped_by_user`): The user clicked Stop to abort this main-agent or conversation-agent run.",
		"- **Underlying error**: `context canceled`",
		"- **Executed**: 1 model turns",
		"- **This run's duration**: 18.6s",
		"- **Tool running at interruption**: `Compress` (ran for 0s, **did not return a result**)",
	}
	for _, want := range wantLines {
		if !strings.Contains(got, want) {
			t.Fatalf("expected %q in DisplayDetail output:\n%s", want, got)
		}
	}
}

func TestDisplayDetailTranslatesAllAbortCauses(t *testing.T) {
	t.Cleanup(func() { config.CurrentLanguage = nil })
	config.CurrentLanguage = func() string { return "en" }

	causes := []*agent.AbortCause{
		agent.AbortPausedByUser,
		agent.AbortPausedByOrchestrator,
		agent.AbortTaskDeleted,
		agent.AbortPausedOnReload,
		agent.AbortGoalMet,
		agent.AbortSettleDrainTimeout,
		agent.AbortKilledByPlanner,
		agent.AbortWorkPausedByUser,
		agent.AbortWorkCancelledByUser,
		agent.AbortWorkFinished,
		agent.AbortPausedRaceGuard,
		agent.AbortChatStoppedByUser,
		agent.AbortChatPausedWithTask,
		agent.AbortChatTurnFinished,
		agent.AbortShutdown,
		agent.AbortRunHardTimeout,
		agent.Causef("queued_for_admission", "任务等待运行准入",
			"任务正在等待并发队列或准入状态提交，本次执行已停止；只有获得运行槽后才会重新领取意图"),
		agent.Causef("llm_unavailable_queued", "LLM 不可用，任务进入等待队列",
			"任务当前无法解析可运行的 Planner/Worker LLM，已释放并发槽；配置恢复后按队列顺序继续"),
	}

	for _, c := range causes {
		block := strings.Join([]string{
			"（运行被中断：" + c.Short + "；停在执行过程中，未完成）",
			"",
			"- **终态**: `aborted_tools` - 运行在工具执行阶段被取消",
			"- **中断原因** (`" + c.Code + "`): " + c.Text,
		}, "\n")
		got := agent.DisplayDetail(block)
		if containsHan(got) {
			t.Fatalf("abort cause %q still contains Han characters:\n%s", c.Code, got)
		}
	}

	// Deadline exceeded carries dynamic error suffix after a fixed Chinese prefix.
	deadlineBlock := strings.Join([]string{
		"（运行被中断：上游 context 到达 deadline；停在执行过程中，未完成）",
		"",
		"- **终态**: `aborted_tools` - 运行在工具执行阶段被取消",
		"- **中断原因** (`deadline_exceeded`): 上游 context 到达 deadline，但设置方没有通过 WithTimeoutCause 附加具名原因: context deadline exceeded",
	}, "\n")
	gotDeadline := agent.DisplayDetail(deadlineBlock)
	if containsHan(gotDeadline) || !strings.Contains(gotDeadline, "context deadline exceeded") {
		t.Fatalf("deadline_exceeded translation unexpected:\n%s", gotDeadline)
	}
}

func TestActivityDTOTranslatesLLMSwitchAndNumericPatterns(t *testing.T) {
	t.Cleanup(func() { config.CurrentLanguage = nil })
	config.CurrentLanguage = func() string { return "en" }

	failover := activityDTO(db.Activity{
		Kind:    "llm_switch",
		Summary: "默认配置 额度不足，后续调用切换到 配置 #2（openai / gpt-5）",
		Detail:  "HTTP 429 rate limit",
	})
	if want := "Out of quota for Default configuration; later calls switch to Configuration #2 (openai / gpt-5)"; failover.Summary != want {
		t.Fatalf("failover summary = %q, want %q", failover.Summary, want)
	}
	if failover.Detail != "HTTP 429 rate limit" {
		t.Fatalf("failover detail should stay intact, got %q", failover.Detail)
	}

	exhausted := activityDTO(db.Activity{
		Kind:    "llm_failover",
		Summary: "默认配置 额度不足，配置链已耗尽",
	})
	if want := "Out of quota for Default configuration; the configuration chain is exhausted"; exhausted.Summary != want {
		t.Fatalf("exhausted summary = %q, want %q", exhausted.Summary, want)
	}

	manual := activityDTO(db.Activity{
		Kind:    "llm_switch",
		Summary: "用户手动将任务 LLM 从 默认配置（anthropic / claude） 切换到 配置 #3（openai / gpt-5）",
		Detail:  "用户手动将任务 LLM 从 默认配置（anthropic / claude） 切换到 配置 #3（openai / gpt-5）",
	})
	wantManual := "The task LLM was switched manually from Default configuration (anthropic / claude) to Configuration #3 (openai / gpt-5)"
	if manual.Summary != wantManual || manual.Detail != wantManual {
		t.Fatalf("manual switch = (%q, %q), want %q", manual.Summary, manual.Detail, wantManual)
	}

	// Regression guard: %d verbs in activitySummaryPatterns must not emit %!d(string=...).
	round := activityDTO(db.Activity{Kind: "text", Summary: "7차 계획 수립"}).Summary
	if round != "Planning round 7" {
		t.Fatalf("round summary = %q, want %q", round, "Planning round 7")
	}
	approval := activityDTO(db.Activity{Kind: "intercept_request", Summary: "도구 Bash 승인 요청 (#42)"}).Summary
	if approval != "Tool Bash approval request (#42)" {
		t.Fatalf("approval summary = %q, want %q", approval, "Tool Bash approval request (#42)")
	}
	ruleMode := activityDTO(db.Activity{
		Kind:    "text",
		Summary: "(규칙 모드, LLM 미설정) 현재 현황: 자산 3개, 대기 의도 4개, 확인된 취약점 1개.\n사용 가능한 명령: \"의도 ...\"로 의도를 주입하고, \"힌트 ...\"로 플래너에게 힌트를 전달합니다.",
	}).Summary
	if strings.Contains(ruleMode, "%!d") || !strings.Contains(ruleMode, "3 assets, 4 pending intents, 1 confirmed vulnerabilities") {
		t.Fatalf("rule mode summary = %q", ruleMode)
	}

	interceptErr := activityDTO(db.Activity{
		Kind:    "intercept_request",
		Summary: "拦截规则读取失败，按失败安全策略转人工确认：permission denied",
	}).Summary
	if want := "Reading the interception rules failed; falling back to human confirmation (fail-safe): permission denied"; interceptErr != want {
		t.Fatalf("intercept err summary = %q, want %q", interceptErr, want)
	}
}
