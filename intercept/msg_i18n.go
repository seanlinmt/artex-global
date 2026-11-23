package intercept

import (
	"errors"
	"fmt"

	"github.com/Autumn-27/artex/config"
)

// 이 포크는 사용자 대면 출력을 선택한 언어로 강제한다. intercept 패키지의 판정·승인
// 문구는 승인 기록(reason·audit.decision_reason), 활동 스트림(intercept_request 카드),
// 409 응답으로 UI 에 그대로 노출되므로 번역 표를 거쳐야 한다. server 패키지의
// errCatalog 를 재사용할 수 없는 이유는 순환 임포트(server → intercept)이고, 그래서
// selfupdate/msg_i18n.go·intercept/label_i18n.go 와 같은 방식으로 이 패키지 안에 자체
// 표를 둔다. 키는 소스의 한국어 원문 그대로다.
//
// 번역하지 않는 것(두뇌 보존):
//   - judge 프롬프트 본문과 모델이 쓴 사유: 모델이 읽고 쓰는 텍스트라 원문 유지.
//   - "[模型]" 센티넬: decision_source 분류·UI 접두 제거·db SQL LIKE 가 의존하는
//     안정 식별자다.
//   - 규칙 이름(r.Name)과 도구 이름: 사용자가 지정한 값·식별자라 번역 대상이 아니다.
//
// 포맷 문자열(%s/%d/%w)은 치환값을 넣기 전에 템플릿 자체를 먼저 번역해야 동사가
// 살아남는다. 테스트가 각 번역이 원문의 동사를 같은 순서로 유지하는지 고정한다.

// interceptMsgCatalog 는 한국어 원문 → 언어별 번역. "ko" 는 키 자체라 저장하지 않는다.
var interceptMsgCatalog = map[string]map[string]string{
	// ---- 판정 결과·폴백 사유 (Judge) ----
	"검토 맥락이 완전하지 않아 사람의 확인이 필요합니다: ": {
		"en": "The review context is incomplete, so human confirmation is required: ",
		"zh": "审查上下文不完整，需要人工确认：",
		"es": "El contexto de revisión está incompleto, por lo que se requiere confirmación humana: ",
		"ja": "レビューの文脈が不完全なため、人による確認が必要です: ",
	},
	"모델 승인에 실패하여 실패 정책에 따라 처리합니다: ": {
		"en": "Model approval failed; the failure policy is applied: ",
		"zh": "模型审批失败，按失败策略处理：",
		"es": "La aprobación del modelo falló; se aplica la política de fallo: ",
		"ja": "モデル承認に失敗したため、失敗ポリシーに従って処理します: ",
	},
	"모델 출력을 해석할 수 없어 실패 정책에 따라 처리합니다": {
		"en": "The model output could not be parsed; the failure policy is applied",
		"zh": "无法解析模型输出，按失败策略处理",
		"es": "No se pudo interpretar la salida del modelo; se aplica la política de fallo",
		"ja": "モデル出力を解釈できないため、失敗ポリシーに従って処理します",
	},
	// 규칙 조회 실패 시 실패 안전(fail-safe) 폴백.
	"拦截规则读取失败，按失败安全策略转人工确认：": {
		"en": "Reading the interception rules failed; falling back to human confirmation (fail-safe): ",
		"zh": "拦截规则读取失败，按失败安全策略转人工确认：",
		"es": "No se pudieron leer las reglas de interceptación; se pasa a confirmación humana (a prueba de fallos): ",
		"ja": "インターセプトルールの読み取りに失敗したため、フェイルセーフ方針で人による確認に切り替えます: ",
	},

	// ---- 활동 스트림 승인 카드 ----
	"도구 %s 승인 요청 (#%d)": {
		"en": "Tool %s approval request (#%d)",
		"zh": "工具 %s 审批请求 (#%d)",
		"es": "Solicitud de aprobación para la herramienta %s (#%d)",
		"ja": "ツール %s の承認リクエスト (#%d)",
	},

	// ---- 규칙 기본 메시지 (defaultMessage) ----
	"가로채기 규칙 [": {
		"en": "Interception rule [",
		"zh": "拦截规则 [",
		"es": "Regla de interceptación [",
		"ja": "インターセプトルール [",
	},
	"] 에 따라 이 도구를 실행할 수 없습니다": {
		"en": "] forbids running this tool",
		"zh": "] 禁止执行此工具",
		"es": "] impide ejecutar esta herramienta",
		"ja": "] により、このツールは実行できません",
	},
	"] 에 따라 사용자 승인이 필요합니다. 잠시 기다려 주세요": {
		"en": "] requires user approval. Please wait",
		"zh": "] 需要用户审批，请稍候",
		"es": "] requiere aprobación del usuario. Espere",
		"ja": "] のためユーザーの承認が必要です。しばらくお待ちください",
	},

	// ---- 승인 기록에 남는 결정 사유 (HandleAsk / Decide) ----
	"사람이 실행을 허용했습니다": {
		"en": "A human allowed the execution",
		"zh": "人工已允许执行",
		"es": "Una persona permitió la ejecución",
		"ja": "人が実行を許可しました",
	},
	"사람이 실행을 거부했습니다": {
		"en": "A human denied the execution",
		"zh": "人工已拒绝执行",
		"es": "Una persona rechazó la ejecución",
		"ja": "人が実行を拒否しました",
	},
	"작업이 취소되었습니다": {
		"en": "The task was cancelled",
		"zh": "任务已取消",
		"es": "La tarea se canceló",
		"ja": "タスクはキャンセルされました",
	},
	"실행 전에 작업이 취소되었습니다": {
		"en": "The task was cancelled before execution",
		"zh": "执行前任务已取消",
		"es": "La tarea se canceló antes de la ejecución",
		"ja": "実行前にタスクがキャンセルされました",
	},
	"승인 시간이 초과되어 시간 초과 정책에 따라 처리합니다": {
		"en": "The approval timed out; the timeout policy is applied",
		"zh": "审批超时，按超时策略处理",
		"es": "La aprobación agotó el tiempo de espera; se aplica la política de tiempo de espera",
		"ja": "承認がタイムアウトしたため、タイムアウトポリシーに従って処理します",
	},

	// ---- 409 응답 (Decide) ----
	"이미 처리되었거나 존재하지 않는 승인입니다. 기록을 새로 고쳐 주세요": {
		"en": "This approval was already resolved or does not exist. Refresh the record and try again",
		"zh": "该审批已处理或不存在，请刷新记录后重试",
		"es": "Esta aprobación ya se resolvió o no existe. Actualice el registro e inténtelo de nuevo",
		"ja": "この承認は既に処理済みか存在しません。記録を更新してから再試行してください",
	},
}

// trMsg 는 한국어 원문을 활성 출력 언어로 치환한다. 표에 없거나 한국어이면 원문 그대로.
func trMsg(ko string) string {
	if m, ok := interceptMsgCatalog[ko]; ok {
		if v, ok2 := m[config.ActiveLanguage()]; ok2 && v != "" {
			return v
		}
	}
	return ko
}

// errf 는 fmt.Errorf 와 같지만 포맷 문자열을 먼저 번역한다.
func errf(format string, args ...any) error {
	return fmt.Errorf(trMsg(format), args...)
}

// errs 는 errors.New 와 같지만 메시지를 먼저 번역한다.
func errs(msg string) error {
	return errors.New(trMsg(msg))
}

// msgf 는 fmt.Sprintf 와 같지만 포맷 문자열을 먼저 번역한다.
func msgf(format string, args ...any) string {
	return fmt.Sprintf(trMsg(format), args...)
}

// ruleMessage 은 규칙 기본 메시지의 접두/접미 조각을 번역해 규칙 이름과 결합한다.
// 규칙 이름은 사용자 지정 값이라 번역하지 않는다.
func ruleMessage(action, ruleName string) string {
	switch action {
	case "deny":
		return trMsg("가로채기 규칙 [") + ruleName + trMsg("] 에 따라 이 도구를 실행할 수 없습니다")
	case "ask":
		return trMsg("가로채기 규칙 [") + ruleName + trMsg("] 에 따라 사용자 승인이 필요합니다. 잠시 기다려 주세요")
	default:
		return ""
	}
}
