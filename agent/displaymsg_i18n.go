package agent

import (
	"regexp"
	"strings"

	"github.com/Autumn-27/artex/config"
)

// Worker terminal blocks (terminalreason.go) and the capture-time error summary
// (capture.go) are written into the activity stream as kind='result' rows. They
// are stored in the ORIGINAL Chinese on purpose:
//
//   - A 'result' row is agent input: the planner reads a worker's result back
//     through get_worker_output / workerOutput, so the stored text is part of the
//     benchmarked brain boundary (段 [A]). Translating at write time would change
//     what the model reads.
//   - Storage must stay language-neutral: a run recorded under one display
//     language has to render in whatever language is selected later, and
//     switching back and forth must not rewrite history.
//
// So the translation happens ONLY when the row is served to the UI. Every
// function here is a display filter: it maps the known framing lines and the
// known reason hints, and returns anything it does not recognize untouched. That
// fail-open rule is what keeps upstream-sync safe — if an upstream change alters
// a source string, the new text renders verbatim (Chinese) instead of breaking,
// and TestTerminalHintTranslations pins the exact strings so re-adding one is a
// one-line fix rather than a retranslation of the whole table.

// terminalDisplayCatalog maps a Chinese source string to its en/ko/es/ja
// translations. Keys are byte-exact source literals; "zh" is the key itself and
// is not stored.
var terminalDisplayCatalog = map[string]map[string]string{
	// ---- reasonHint (terminalreason.go) ----
	"模型正常结束了本轮，但没有留下文字总结；事实和资产以本轮工具调用记录为准": {
		"en": "The model ended this round normally but left no written summary; facts and assets are taken from this round's tool-call record",
		"ko": "모델이 이번 라운드를 정상적으로 끝냈지만 글로 된 요약을 남기지 않았습니다. 사실과 자산은 이번 라운드의 도구 호출 기록을 기준으로 합니다",
		"es": "El modelo terminó esta ronda con normalidad pero no dejó un resumen escrito; los hechos y activos se toman del registro de llamadas a herramientas de esta ronda",
		"ja": "モデルはこのラウンドを正常に終えましたが、文章のまとめを残していません。事実とアセットはこのラウンドのツール呼び出し記録に基づきます",
	},
	"达到步数上限(MaxTurns)：SDK 已执行收尾并写回事实和资产，意图会标记为 exhausted，供规划者换方向继续，而不是作为失败处理": {
		"en": "Step limit reached (MaxTurns): the SDK wrapped up and wrote back facts and assets; the intent is marked exhausted so the planner can continue in another direction rather than treating it as a failure",
		"ko": "단계 상한(MaxTurns)에 도달했습니다. SDK 가 마무리를 수행해 사실과 자산을 기록했고, 의도는 exhausted 로 표시되어 플래너가 다른 방향으로 이어갈 수 있습니다(실패로 처리하지 않음)",
		"es": "Se alcanzó el límite de pasos (MaxTurns): el SDK cerró la ejecución y registró hechos y activos; la intención se marca como exhausted para que el planificador continúe en otra dirección en lugar de tratarlo como un fallo",
		"ja": "ステップ上限(MaxTurns)に達しました。SDK が締めくくりを行い、事実とアセットを書き戻しました。インテントは exhausted として記録され、プランナーは失敗扱いせず別の方向で続行できます",
	},
	"达到单次运行的墙钟预算(MaxDuration)：到点会打断在跑的工具并就地进收尾，把已识别的事实和资产写回，意图会标记为 exhausted": {
		"en": "Single-run wall-clock budget reached (MaxDuration): at the deadline any running tool is interrupted and the run wraps up in place, writing back the facts and assets identified so far; the intent is marked exhausted",
		"ko": "단일 실행의 실제 시간 예산(MaxDuration)에 도달했습니다. 시각이 되면 실행 중인 도구를 중단하고 그 자리에서 마무리해, 지금까지 식별한 사실과 자산을 기록하며 의도는 exhausted 로 표시됩니다",
		"es": "Se alcanzó el presupuesto de tiempo real de una ejecución (MaxDuration): al llegar el plazo se interrumpe cualquier herramienta en curso y la ejecución cierra en el sitio, registrando los hechos y activos identificados; la intención se marca como exhausted",
		"ja": "単一実行の実時間予算(MaxDuration)に達しました。期限になると実行中のツールを中断し、その場で締めくくって、判明した事実とアセットを書き戻します。インテントは exhausted として記録されます",
	},
	"模型或 API 调用失败（网络、鉴权、限流、供应商 5xx 等），重试用尽后意图标记为 blocked——传输层故障导致这条意图基本没真正探成；查其执行过程（get_worker_trace）后再决定重派或换法": {
		"en": "The model or API call failed (network, auth, rate limiting, provider 5xx, etc.); after retries were exhausted the intent is marked blocked — a transport-layer fault means this intent was essentially never actually explored; inspect its execution trace (get_worker_trace) before deciding to re-dispatch it or change approach",
		"ko": "모델 또는 API 호출이 실패했습니다(네트워크·인증·속도 제한·공급자 5xx 등). 재시도를 모두 소진한 뒤 의도는 blocked 로 표시됩니다. 전송 계층 장애라 이 의도는 사실상 제대로 탐색되지 않았으므로, 실행 과정(get_worker_trace)을 확인한 뒤 재배정할지 방법을 바꿀지 결정하십시오",
		"es": "La llamada al modelo o a la API falló (red, autenticación, limitación de velocidad, 5xx del proveedor, etc.); tras agotar los reintentos la intención se marca como blocked — un fallo de la capa de transporte significa que esta intención prácticamente no llegó a explorarse; revise su traza de ejecución (get_worker_trace) antes de decidir reenviarla o cambiar de enfoque",
		"ja": "モデルまたは API 呼び出しに失敗しました（ネットワーク・認証・レート制限・プロバイダの 5xx など）。再試行を使い切った後、インテントは blocked として記録されます。伝送層の障害のため、このインテントは実質的に探索できていません。実行過程（get_worker_trace）を確認してから、再投入するか方法を変えるかを決めてください",
	},
	"上下文长度达到硬上限，请求在发出前被拦截；应收窄意图粒度或压缩工具返回": {
		"en": "The context length hit a hard limit, so the request was blocked before being sent; narrow the intent's granularity or compress tool output",
		"ko": "컨텍스트 길이가 하드 상한에 도달해 요청이 전송되기 전에 차단되었습니다. 의도 단위를 좁히거나 도구 반환값을 압축하십시오",
		"es": "La longitud del contexto alcanzó un límite estricto, por lo que la solicitud se bloqueó antes de enviarse; reduzca la granularidad de la intención o comprima la salida de las herramientas",
		"ja": "コンテキスト長がハード上限に達したため、リクエストは送信前にブロックされました。インテントの粒度を狭めるか、ツールの返り値を圧縮してください",
	},
	"提示词过长且上下文压缩重试已经用尽，无法继续执行": {
		"en": "The prompt is too long and the context-compression retries are exhausted, so execution cannot continue",
		"ko": "프롬프트가 너무 길고 컨텍스트 압축 재시도까지 모두 소진되어 더는 실행할 수 없습니다",
		"es": "El prompt es demasiado largo y los reintentos de compresión del contexto están agotados, por lo que no se puede continuar la ejecución",
		"ja": "プロンプトが長すぎ、コンテキスト圧縮の再試行も使い切ったため、これ以上実行できません",
	},
	"当前模型不支持本轮多模态内容；请切换支持视觉的模型或避免工具返回图片": {
		"en": "The current model does not support this round's multimodal content; switch to a vision-capable model or avoid tools returning images",
		"ko": "현재 모델이 이번 라운드의 멀티모달 내용을 지원하지 않습니다. 비전을 지원하는 모델로 바꾸거나 도구가 이미지를 반환하지 않게 하십시오",
		"es": "El modelo actual no admite el contenido multimodal de esta ronda; cambie a un modelo con capacidad de visión o evite que las herramientas devuelvan imágenes",
		"ja": "現在のモデルはこのラウンドのマルチモーダル内容に対応していません。視覚対応のモデルに切り替えるか、ツールが画像を返さないようにしてください",
	},
	"Stop 钩子阻止本轮结束，随后未能继续；请检查任务 Guard 规则是否过严": {
		"en": "A Stop hook prevented this round from finishing and execution then could not continue; check whether the task's Guard rules are too strict",
		"ko": "Stop 훅이 이번 라운드의 종료를 막았고 그 뒤로 계속 진행하지 못했습니다. 작업의 Guard 규칙이 지나치게 엄격한지 확인하십시오",
		"es": "Un hook Stop impidió que esta ronda terminara y después la ejecución no pudo continuar; compruebe si las reglas de Guard de la tarea son demasiado estrictas",
		"ja": "Stop フックがこのラウンドの終了を妨げ、その後続行できませんでした。タスクの Guard ルールが厳しすぎないか確認してください",
	},
	"工具或钩子主动停止继续执行，例如越界目标或禁用命令；请检查最后一条 tool_result 的拦截说明": {
		"en": "A tool or hook actively stopped further execution, for example an out-of-scope target or a disabled command; check the interception note on the last tool_result",
		"ko": "도구 또는 훅이 실행을 주도적으로 중단했습니다(범위를 벗어난 대상이나 비활성화된 명령 등). 마지막 tool_result 의 차단 안내를 확인하십시오",
		"es": "Una herramienta o un hook detuvo activamente la ejecución, por ejemplo un objetivo fuera de alcance o un comando deshabilitado; revise la nota de interceptación del último tool_result",
		"ja": "ツールまたはフックが実行を主体的に停止しました（範囲外のターゲットや無効化されたコマンドなど）。最後の tool_result のブロック説明を確認してください",
	},
	"运行在模型输出流式生成阶段被取消": {
		"en": "The run was cancelled while the model was streaming its output",
		"ko": "모델 출력이 스트리밍되던 단계에서 실행이 취소되었습니다",
		"es": "La ejecución se canceló mientras el modelo transmitía su salida",
		"ja": "モデル出力のストリーミング中に実行がキャンセルされました",
	},
	"运行在工具执行阶段被取消": {
		"en": "The run was cancelled during tool execution",
		"ko": "도구 실행 단계에서 실행이 취소되었습니다",
		"es": "La ejecución se canceló durante la ejecución de herramientas",
		"ja": "ツール実行の段階で実行がキャンセルされました",
	},
	"未知终态；harness 可能新增了 TerminalReason，请补充 reasonHint": {
		"en": "Unknown terminal state; the harness may have added a TerminalReason — add a matching reasonHint",
		"ko": "알 수 없는 종료 상태입니다. harness 에 TerminalReason 이 추가되었을 수 있으니 reasonHint 를 보완하십시오",
		"es": "Estado terminal desconocido; puede que el harness haya añadido un TerminalReason — añada el reasonHint correspondiente",
		"ja": "不明な終了状態です。harness に TerminalReason が追加された可能性があるため、reasonHint を追加してください",
	},
	"运行的 context 已取消，但底层没有产生 Terminal 事件": {
		"en": "The run's context was cancelled, but no Terminal event was produced underneath",
		"ko": "실행의 컨텍스트가 취소되었지만 하위에서 Terminal 이벤트가 생성되지 않았습니다",
		"es": "El contexto de la ejecución se canceló, pero no se produjo ningún evento Terminal por debajo",
		"ja": "実行のコンテキストはキャンセルされましたが、下位で Terminal イベントが発生していません",
	},
	"未能取得取消原因": {
		"en": "could not obtain the cancellation cause",
		"ko": "취소 사유를 얻지 못했습니다",
		"es": "no se pudo obtener la causa de la cancelación",
		"ja": "キャンセル理由を取得できませんでした",
	},

	// ---- abort stages ----
	"执行过程中":  {"en": "during execution", "ko": "실행 도중", "es": "durante la ejecución", "ja": "実行中"},
	"模型输出阶段": {"en": "in the model-output stage", "ko": "모델 출력 단계", "es": "en la fase de salida del modelo", "ja": "モデル出力の段階"},
	"工具执行阶段": {"en": "in the tool-execution stage", "ko": "도구 실행 단계", "es": "en la fase de ejecución de herramientas", "ja": "ツール実行の段階"},

	// ---- summary framing fragments ----
	"（运行被中断：":    {"en": "(run interrupted: ", "ko": "(실행 중단: ", "es": "(ejecución interrumpida: ", "ja": "(実行中断: "},
	"；停在":        {"en": "; stopped ", "ko": "; 중단 지점 ", "es": "; detenida ", "ja": "; 停止位置 "},
	"，未完成）":      {"en": ", incomplete)", "ko": ", 미완료)", "es": ", sin completar)", "ja": "、未完了)"},
	"（达到运行预算上限(": {"en": "(run budget reached (", "ko": "(실행 예산 상한 도달(", "es": "(se alcanzó el límite del presupuesto de ejecución (", "ja": "(実行予算の上限に到達("},
	")，已收尾写回事实":  {"en": "), wrapped up and wrote back facts", "ko": "), 마무리 후 사실 기록 완료", "es": "), se cerró y se registraron los hechos", "ja": ")、締めくくって事実を書き戻しました"},
	"；本次无文字总结）":  {"en": "; no written summary this time)", "ko": "; 이번에는 글로 된 요약 없음)", "es": "; sin resumen escrito esta vez)", "ja": "。今回の文章のまとめはありません)"},
	"（无文字总结，终态 ": {"en": "(no written summary, terminal state ", "ko": "(글로 된 요약 없음, 종료 상태 ", "es": "(sin resumen escrito, estado terminal ", "ja": "(文章のまとめなし、終了状態 "},
	"：":          {"en": ": ", "ko": ": ", "es": ": ", "ja": ": "},
	"）":          {"en": ")", "ko": ")", "es": ")", "ja": ")"},
	"，已运行 ":      {"en": "; ran for ", "ko": "; 실행 시간 ", "es": "; en ejecución ", "ja": "、実行時間 "},
	" 轮":         {"en": " turns", "ko": " 라운드", "es": " rondas", "ja": " ラウンド"},

	// ---- detail bullets ----
	"- **终态**: ":    {"en": "- **Terminal state**: ", "ko": "- **종료 상태**: ", "es": "- **Estado terminal**: ", "ja": "- **終了状態**: "},
	"- **中断原因** (`": {"en": "- **Interruption cause** (`", "ko": "- **중단 사유** (`", "es": "- **Causa de la interrupción** (`", "ja": "- **中断理由** (`"},
	"`): ":          {"en": "`): ", "ko": "`): ", "es": "`): ", "ja": "`): "},
	"- **中断原因**: 无法取得；取消方可能没有通过 context.WithCancelCause 附加具名原因": {
		"en": "- **Interruption cause**: unavailable; the canceller may not have attached a named cause via context.WithCancelCause",
		"ko": "- **중단 사유**: 확인할 수 없습니다. 취소한 쪽이 context.WithCancelCause 로 명명된 사유를 붙이지 않았을 수 있습니다",
		"es": "- **Causa de la interrupción**: no disponible; es posible que quien canceló no adjuntara una causa con nombre mediante context.WithCancelCause",
		"ja": "- **中断理由**: 取得できません。キャンセルした側が context.WithCancelCause で名前付きの理由を付与していない可能性があります",
	},
	"- **底层错误**: `":       {"en": "- **Underlying error**: `", "ko": "- **하위 오류**: `", "es": "- **Error subyacente**: `", "ja": "- **基盤エラー**: `"},
	"- **取消前已生成的部分输出**:":  {"en": "- **Partial output generated before cancellation**:", "ko": "- **취소 전에 생성된 일부 출력**:", "es": "- **Salida parcial generada antes de la cancelación**:", "ja": "- **キャンセル前に生成された一部の出力**:"},
	"- **已执行**: ":         {"en": "- **Executed**: ", "ko": "- **실행**: ", "es": "- **Ejecutado**: ", "ja": "- **実行**: "},
	" 轮模型回合":              {"en": " model turns", "ko": " 라운드 모델 턴", "es": " turnos del modelo", "ja": " ラウンドのモデルターン"},
	"- **本次运行耗时**: ":      {"en": "- **This run's duration**: ", "ko": "- **이번 실행 소요 시간**: ", "es": "- **Duración de esta ejecución**: ", "ja": "- **今回の実行所要時間**: "},
	"- **累计 token**: 输入 ": {"en": "- **Cumulative tokens**: input ", "ko": "- **누적 token**: 입력 ", "es": "- **Tokens acumulados**: entrada ", "ja": "- **累計 token**: 入力 "},
	" / 输出 ":              {"en": " / output ", "ko": " / 출력 ", "es": " / salida ", "ja": " / 出力 "},
	" / 缓存读 ":             {"en": " / cache read ", "ko": " / 캐시 읽기 ", "es": " / lectura de caché ", "ja": " / キャッシュ読取 "},
	" / 缓存写 ":             {"en": " / cache write ", "ko": " / 캐시 쓰기 ", "es": " / escritura de caché ", "ja": " / キャッシュ書込 "},
	"- **工具调用**: 本次运行还没有发出工具调用就结束了": {
		"en": "- **Tool calls**: this run ended before issuing any tool call",
		"ko": "- **도구 호출**: 이번 실행은 도구 호출을 한 번도 보내지 않은 채 끝났습니다",
		"es": "- **Llamadas a herramientas**: esta ejecución terminó sin emitir ninguna llamada a herramientas",
		"ja": "- **ツール呼び出し**: 今回の実行はツール呼び出しを一度も発行せずに終了しました",
	},
	"- **中断时正在执行的工具**: `": {"en": "- **Tool running at interruption**: `", "ko": "- **중단 시점에 실행 중이던 도구**: `", "es": "- **Herramienta en ejecución al interrumpirse**: `", "ja": "- **中断時に実行中だったツール**: `"},
	"（已运行 ":              {"en": " (ran for ", "ko": " (실행 시간 ", "es": " (en ejecución ", "ja": " (実行時間 "},
	"，**未返回结果**）":        {"en": ", **did not return a result**)", "ko": ", **결과를 반환하지 않음**)", "es": ", **no devolvió resultado**)", "ja": "、**結果を返していません**)"},
	"- **中断前最后一个工具**: `": {"en": "- **Last tool before interruption**: `", "ko": "- **중단 직전 마지막 도구**: `", "es": "- **Última herramienta antes de la interrupción**: `", "ja": "- **中断前の最後のツール**: `"},
	"（已正常返回）":            {"en": " (returned normally)", "ko": " (정상 반환됨)", "es": " (devolvió con normalidad)", "ja": "（正常に返りました）"},

	// ---- capture.go ----
	"执行出错: ": {"en": "Execution error: ", "ko": "실행 오류: ", "es": "Error de ejecución: ", "ja": "実行エラー: "},

	// ---- cancelcause.go & server/goals.go abort causes (short + text) ----
	"用户暂停了任务": {
		"en": "user paused the task",
		"ko": "사용자가 작업을 일시중지했습니다",
		"es": "el usuario pausó la tarea",
		"ja": "ユーザーがタスクを一時停止しました",
	},
	"用户通过任务控制接口（POST /api/tasks/{id}/control，action=pause）暂停了任务。本次 Planner/Worker 运行被主动取消；运行中的意图会退回 frontier(open)，恢复任务后重新领取并从头执行": {
		"en": "The user paused the task via the task control API (POST /api/tasks/{id}/control, action=pause). This Planner/Worker run was cancelled; in-flight intents return to frontier(open) and will be re-claimed from the start when resumed",
		"ko": "사용자가 작업 제어 API(POST /api/tasks/{id}/control, action=pause)로 작업을 일시중지했습니다. 이번 Planner/Worker 실행이 취소되었으며, 실행 중이던 의도는 frontier(open) 로 돌아가 재개 후 처음부터 다시 실행됩니다",
		"es": "El usuario pausó la tarea mediante la API de control de tareas (POST /api/tasks/{id}/control, action=pause). Esta ejecución de Planner/Worker se canceló; las intenciones en curso vuelven a frontier(open) y se retomarán desde el inicio al reanudar",
		"ja": "ユーザーがタスク制御 API（POST /api/tasks/{id}/control, action=pause）でタスクを一時停止しました。今回の Planner/Worker の実行はキャンセルされ、実行中のインテントは frontier(open) に戻り、再開後に最初から再実行されます",
	},
	"编排 Agent 暂停了任务": {
		"en": "orchestrator agent paused the task",
		"ko": "오케스트레이션 에이전트가 작업을 일시중지했습니다",
		"es": "el agente orquestador pausó la tarea",
		"ja": "オーケストレーターエージェントがタスクを一時停止しました",
	},
	"编排 Agent 调用了 pause_task 工具暂停本任务。本次 Planner/Worker 运行被主动取消；运行中的意图会退回 frontier(open)，恢复后重新执行": {
		"en": "The orchestrator agent called pause_task to pause this task. This Planner/Worker run was cancelled; in-flight intents return to frontier(open) and will re-run when resumed",
		"ko": "오케스트레이션 에이전트가 pause_task 도구를 호출해 이 작업을 일시중지했습니다. 이번 Planner/Worker 실행이 취소되었으며, 실행 중이던 의도는 frontier(open) 로 돌아가 재개 후 다시 실행됩니다",
		"es": "El agente orquestador llamó a la herramienta pause_task para pausar esta tarea. Esta ejecución de Planner/Worker se canceló; las intenciones en curso vuelven a frontier(open) y se volverán a ejecutar al reanudar",
		"ja": "オーケストレーターエージェントが pause_task ツールを呼び出してこのタスクを一時停止しました。今回の Planner/Worker の実行はキャンセルされ、実行中のインテントは frontier(open) に戻り、再開後に再実行されます",
	},
	"任务被删除": {
		"en": "task was deleted",
		"ko": "작업이 삭제되었습니다",
		"es": "la tarea fue eliminada",
		"ja": "タスクが削除されました",
	},
	"任务正在删除（DELETE /api/tasks/{id}），删除屏障已取消该任务正在运行的 Planner、Worker 和主 Agent；本次运行结果不会再被使用": {
		"en": "The task is being deleted (DELETE /api/tasks/{id}); the deletion barrier cancelled the running Planner, Worker, and main agent for this task, and this run's output will not be used",
		"ko": "작업을 삭제하는 중(DELETE /api/tasks/{id})이므로 삭제 장벽이 실행 중인 Planner, Worker, 메인 에이전트를 취소했습니다. 이번 실행 결과는 사용되지 않습니다",
		"es": "La tarea se está eliminando (DELETE /api/tasks/{id}); la barrera de eliminación canceló el Planner, Worker y agente principal en ejecución para esta tarea, y el resultado de esta ejecución no se utilizará",
		"ja": "タスクを削除中（DELETE /api/tasks/{id}）のため、削除バリアが実行中の Planner、Worker、メインエージェントをキャンセルしました。この実行結果は使用されません",
	},
	"后端恢复了任务的暂停状态": {
		"en": "backend restored the task's paused state",
		"ko": "백엔드가 작업의 일시중지 상태를 복원했습니다",
		"es": "el backend restauró el estado en pausa de la tarea",
		"ja": "バックエンドがタスクの一時停止状態を復元しました",
	},
	"后端启动时根据数据库中持久化的状态恢复了任务暂停。本次运行被取消；正常情况下恢复阶段没有正在运行的 Agent": {
		"en": "At startup, the backend restored the task's paused state from the database. This run was cancelled; normally no agent is running during recovery",
		"ko": "백엔드 시작 시 데이터베이스에 저장된 상태에 따라 작업 일시중지를 복원했습니다. 이번 실행은 취소되었으며, 정상적인 복구 단계에서는 실행 중인 에이전트가 없습니다",
		"es": "Al iniciar, el backend restauró el estado en pausa de la tarea desde la base de datos. Esta ejecución se canceló; normalmente no hay ningún agente ejecutándose durante la recuperación",
		"ja": "起動時にバックエンドがデータベースに永続化された状態からタスクの一時停止を復元しました。今回の実行はキャンセルされました。通常、復元段階では実行中のエージェントはありません",
	},
	"规划者判定任务目标已达成": {
		"en": "planner judged that task goals are met",
		"ko": "플래너가 작업 목표가 달성되었다고 판단했습니다",
		"es": "el planificador determinó que se cumplieron los objetivos de la tarea",
		"ja": "プランナーがタスク目標の達成を判定しました",
	},
	"规划者判定任务目标已达成并将任务置为 done，随后取消仍在运行的 Worker；这些意图会标记为 stopped，而不是失败": {
		"en": "The planner judged that all task goals were met, marked the task done, and cancelled any still-running Workers; those intents are marked stopped rather than failed",
		"ko": "플래너가 작업 목표를 모두 달성했다고 판단해 작업을 done 으로 표시하고, 계속 실행 중이던 Worker 를 취소했습니다. 해당 의도는 실패가 아닌 stopped 로 표시됩니다",
		"es": "El planificador determinó que se cumplieron todos los objetivos, marcó la tarea como done y canceló los Workers que seguían en ejecución; esas intenciones se marcan como stopped en lugar de fallidas",
		"ja": "プランナーがタスク目標の達成を判定してタスクを done にし、実行中の Worker をキャンセルしました。これらのインテントは失敗ではなく stopped として記録されます",
	},
	"任务超时收尾的等待时间已用尽": {
		"en": "task timeout drain grace period expired",
		"ko": "작업 시간 초과 마무리 대기 시간이 만료되었습니다",
		"es": "expiró el tiempo de espera de cierre por tiempo agotado de la tarea",
		"ja": "タスクのタイムアウト終了処理の待機時間が尽きました",
	},
	"任务到达 timeout 后等待正在运行的 Worker 优雅收尾，但 90 秒 drain 宽限仍不足，因此执行硬取消；意图会标记为 exhausted，收尾阶段已经写入的事实和资产会保留": {
		"en": "After the task hit its timeout, the 90-second drain grace period for running Workers to wrap up gracefully expired, so a hard cancellation was issued; the intent is marked exhausted and facts/assets already written during wrap-up are preserved",
		"ko": "작업이 timeout 에 도달한 뒤 실행 중인 Worker 의 정상 마무리를 기다렸으나 90초 유예 시간이 지나 강제 취소했습니다. 의도는 exhausted 로 표시되며 마무리 중 기록된 사실과 자산은 보존됩니다",
		"es": "Tras alcanzar el tiempo límite de la tarea, expiró el periodo de gracia de 90 segundos para el cierre ordenado de los Workers en ejecución, por lo que se aplicó una cancelación forzada; la intención se marca como exhausted y se conservan los hechos y activos ya registrados",
		"ja": "タスクが timeout に達した後、実行中の Worker の正常終了を待ちましたが、90 秒のドレイン猶予時間を超えたため強制キャンセルされました。インテントは exhausted となり、終了処理中に書き込まれた事実とアセットは保持されます",
	},
	"规划者终止了这条意图": {
		"en": "planner killed this intent",
		"ko": "플래너가 이 의도를 종료했습니다",
		"es": "el planificador detuvo esta intención",
		"ja": "プランナーがこのインテントを停止しました",
	},
	"规划者调用 kill_work 主动终止了这条意图，通常表示方向跑偏或已无继续价值；意图会标记为 stopped，不会自动重新领取": {
		"en": "The planner called kill_work to stop this intent, usually because the direction went off-track or is no longer worth pursuing; the intent is marked stopped and will not be automatically re-claimed",
		"ko": "플래너가 kill_work 를 호출해 이 의도를 종료했습니다(보통 방향이 빗나갔거나 계속할 가치가 없을 때). 의도는 stopped 로 표시되며 자동으로 다시 할당되지 않습니다",
		"es": "El planificador llamó a kill_work para detener esta intención, normalmente porque se desvió o ya no vale la pena continuar; la intención se marca como stopped y no se volverá a reclamar automáticamente",
		"ja": "プランナーが kill_work を呼び出してこのインテントを停止しました（通常、方向性がずれたか継続の価値がないため）。インテントは stopped と記録され、自動再取得されません",
	},
	"用户暂停了这条 Worker 意图": {
		"en": "user paused this Worker intent",
		"ko": "사용자가 이 Worker 의도를 일시중지했습니다",
		"es": "el usuario pausó esta intención de Worker",
		"ja": "ユーザーがこの Worker インテントを一時停止しました",
	},
	"用户暂停了正在运行的 Worker。本次调用被取消，意图转为 paused；已经登记的意图、事实、漏洞和活动记录全部保留，恢复后从头重新执行": {
		"en": "The user paused the running Worker. This call was cancelled and the intent moved to paused; already-recorded intents, facts, findings, and activity records are preserved, and execution restarts from the beginning on resume",
		"ko": "사용자가 실행 중인 Worker 를 일시중지했습니다. 이번 호출이 취소되어 의도는 paused 로 전환되었으며, 이미 기록된 의도·사실·취약점·활동 기록은 모두 보존되고 재개 시 처음부터 다시 실행됩니다",
		"es": "El usuario pausó el Worker en ejecución. Se canceló esta llamada y la intención pasó a paused; se conservan las intenciones, hechos, hallazgos y registros de actividad ya guardados, y al reanudar se ejecuta de nuevo desde el principio",
		"ja": "ユーザーが実行中の Worker を一時停止しました。この呼び出しはキャンセルされ、インテントは paused に移行しました。記録済みのインテント、事実、脆弱性、アクティビティ記録はすべて保持され、再開時は最初から再実行されます",
	},
	"用户删除了这条 Worker 意图": {
		"en": "user deleted this Worker intent",
		"ko": "사용자가 이 Worker 의도를 삭제했습니다",
		"es": "el usuario eliminó esta intención de Worker",
		"ja": "ユーザーがこの Worker インテントを削除しました",
	},
	"用户删除了正在运行的 Worker。本次调用被取消；Worker 退出写入区后，服务端按用户选择的删除模式处理该意图——假删除仅标记为已删除并保留全部产出，真删除会级联移除该意图及仅由它支撑的下游节点": {
		"en": "The user deleted the running Worker. This call was cancelled; once the Worker leaves the write section, the server applies the chosen deletion mode — soft delete marks it deleted and preserves all outputs, while hard delete cascades to this intent and downstream nodes supported only by it",
		"ko": "사용자가 실행 중인 Worker 를 삭제했습니다. 이번 호출이 취소되었으며, Worker 가 쓰기 구간을 빠져나오면 서버가 선택한 삭제 모드로 처리합니다(소프트 삭제는 삭제 표시만 하고 모든 산출물을 보존, 영구 삭제는 이 의도와 이 의도에만 의존하는 하위 노드를 연쇄 제거)",
		"es": "El usuario eliminó el Worker en ejecución. Se canceló esta llamada; cuando el Worker salga de la sección de escritura, el servidor aplica el modo de eliminación elegido: la eliminación lógica la marca como eliminada conservando sus resultados, y la eliminación definitiva elimina en cascada esta intención y los nodos posteriores que solo dependen de ella",
		"ja": "ユーザーが実行中の Worker を削除しました。この呼び出しはキャンセルされ、Worker が書き込み区間を抜けた後、サーバーが選択された削除モードで処理します（論理削除は削除済みマークのみで成果物をすべて保持、完全削除はこのインテントとそれのみに依存する下流ノードを連鎖削除）",
	},
	"Worker 已正常结束并释放 context": {
		"en": "Worker finished normally and released its context",
		"ko": "Worker 가 정상 종료되고 context 를 해제했습니다",
		"es": "el Worker finalizó con normalidad y liberó su context",
		"ja": "Worker は正常終了し context を解放しました",
	},
	"Worker 已正常结束，引擎在 detachWork 中释放其 context 资源。这不是运行中断；若它出现在中断消息中，说明取消与收场事件发生了竞态": {
		"en": "The Worker finished normally and the engine released its context in detachWork. This is not an interruption; if it appears in an interruption message, a race occurred between cancellation and teardown",
		"ko": "Worker 가 정상 종료되어 엔진이 detachWork 에서 context 자원을 해제했습니다. 실행 중단이 아니며, 중단 메시지에 나타났다면 취소와 정리 이벤트 사이에 경합이 발생한 것입니다",
		"es": "El Worker terminó con normalidad y el motor liberó su context en detachWork. No es una interrupción; si aparece en un mensaje de interrupción, hubo una condición de carrera entre la cancelación y el cierre",
		"ja": "Worker は正常終了し、エンジンが detachWork で context リソースを解放しました。これは中断ではありません。中断メッセージに現れた場合、キャンセルと終了処理の間で競合が発生しています",
	},
	"任务暂停期间拒绝启动新运行": {
		"en": "refused to start a new run while the task is paused",
		"ko": "작업 일시중지 중 새 실행 시작이 거부되었습니다",
		"es": "se rechazó iniciar una nueva ejecución mientras la tarea está en pausa",
		"ja": "タスクの一時停止中は新しい実行の開始を拒否しました",
	},
	"任务处于暂停状态时，引擎拒绝发出新的执行 context，用于防止 claim 与暂停之间的竞态导致 Worker 继续启动；已领取的意图会退回 frontier": {
		"en": "While the task is paused, the engine refuses to issue a new execution context to prevent a race between claim and pause from launching another Worker; claimed intents return to the frontier",
		"ko": "작업이 일시중지 상태일 때 claim 과 일시중지 사이의 경합으로 Worker 가 계속 시작되는 것을 막기 위해 엔진이 새 실행 context 발급을 거부했습니다. 할당된 의도는 frontier 로 돌아갑니다",
		"es": "Mientras la tarea está en pausa, el motor rechaza emitir un nuevo context de ejecución para evitar que una carrera entre claim y pausa inicie otro Worker; las intenciones reclamadas vuelven al frontier",
		"ja": "タスクが一時停止中のため、claim と一時停止の競合で別の Worker が起動するのを防ぐべく、エンジンが新しい実行 context の発行を拒否しました。取得済みのインテントは frontier に戻ります",
	},
	"用户停止了本轮对话": {
		"en": "user stopped this conversation turn",
		"ko": "사용자가 이번 대화를 중지했습니다",
		"es": "el usuario detuvo este turno de conversación",
		"ja": "ユーザーが今回の会話を停止しました",
	},
	"用户点击了停止，主动中止本轮主 Agent 或会话 Agent 运行。已经产生的活动记录会保留，可以继续发送下一条消息": {
		"en": "The user clicked Stop to abort this main-agent or conversation-agent run. Activity records already produced are preserved, and you can continue sending the next message",
		"ko": "사용자가 중지를 클릭해 이번 메인 에이전트 또는 대화 에이전트 실행을 중단했습니다. 이미 생성된 활동 기록은 보존되며, 다음 메시지를 계속 보낼 수 있습니다",
		"es": "El usuario hizo clic en Detener para abortar esta ejecución del agente principal o de conversación. Los registros de actividad ya generados se conservan y puede seguir enviando el siguiente mensaje",
		"ja": "ユーザーが停止をクリックし、今回のメインエージェントまたは会話エージェントの実行を中止しました。すでに生成されたアクティビティ記録は保持され、次のメッセージを続けて送信できます",
	},
	"任务暂停并中止了主 Agent 对话": {
		"en": "task paused and aborted the main-agent conversation",
		"ko": "작업 일시중지로 메인 에이전트 대화가 중단되었습니다",
		"es": "la tarea se pausó y abortó la conversación del agente principal",
		"ja": "タスクの一時停止によりメインエージェントの会話が中止されました",
	},
	"用户暂停任务时，正在运行的主 Agent 对话也被同步取消。已经产生的活动记录会保留；恢复任务后不会自动重放本轮消息": {
		"en": "When the user paused the task, the running main-agent conversation was also cancelled. Activity records already produced are preserved; this message will not be replayed automatically when the task resumes",
		"ko": "사용자가 작업을 일시중지하면서 실행 중이던 메인 에이전트 대화도 함께 취소되었습니다. 이미 생성된 활동 기록은 보존되며, 작업을 재개해도 이번 메시지는 자동으로 다시 실행되지 않습니다",
		"es": "Al pausar el usuario la tarea, también se canceló la conversación en curso del agente principal. Los registros de actividad ya generados se conservan; este mensaje no se volverá a reproducir automáticamente al reanudar la tarea",
		"ja": "ユーザーがタスクを一時停止した際、実行中のメインエージェントの会話も同時にキャンセルされました。すでに生成されたアクティビティ記録は保持され、タスク再開時にこのメッセージが自動再送されることはありません",
	},
	"本轮对话已正常结束并释放 context": {
		"en": "conversation turn finished normally and released its context",
		"ko": "이번 대화가 정상 종료되고 context 를 해제했습니다",
		"es": "el turno de conversación finalizó con normalidad y liberó su context",
		"ja": "今回の会話は正常終了し context を解放しました",
	},
	"本轮对话已正常结束，服务端正在释放该轮 context 资源。这不是运行中断；若它出现在中断消息中，说明取消与收场事件发生了竞态": {
		"en": "This conversation turn finished normally and the server is releasing its context resources. This is not an interruption; if it appears in an interruption message, a race occurred between cancellation and teardown",
		"ko": "이번 대화가 정상 종료되어 서버가 해당 턴의 context 자원을 해제하고 있습니다. 실행 중단이 아니며, 중단 메시지에 나타났다면 취소와 정리 이벤트 사이에 경합이 발생한 것입니다",
		"es": "Este turno de conversación finalizó con normalidad y el servidor está liberando sus recursos de context. No es una interrupción; si aparece en un mensaje de interrupción, hubo una condición de carrera entre la cancelación y el cierre",
		"ja": "今回の会話は正常終了し、サーバーがこのターンの context リソースを解放しています。これは中断ではありません。中断メッセージに現れた場合、キャンセルと終了処理の間で競合が発生しています",
	},
	"后端进程正在关闭": {
		"en": "backend process is shutting down",
		"ko": "백엔드 프로세스가 종료 중입니다",
		"es": "el proceso backend se está cerrando",
		"ja": "バックエンドプロセスがシャットダウン中です",
	},
	"后端进程收到 SIGINT 或 SIGTERM，正在重启、更新或关闭。所有运行中的 Agent 会被取消；重启后残留的 running 意图会重置为 open 并重新执行": {
		"en": "The backend process received SIGINT or SIGTERM and is restarting, updating, or shutting down. All running agents are cancelled; any intents left in running state after restart are reset to open and re-executed",
		"ko": "백엔드 프로세스가 SIGINT 또는 SIGTERM 을 받아 재시작·업데이트·종료 중입니다. 실행 중인 모든 에이전트가 취소되며, 재시작 후 남은 running 상태의 의도는 open 으로 초기화되어 다시 실행됩니다",
		"es": "El proceso backend recibió SIGINT o SIGTERM y se está reiniciando, actualizando o cerrando. Todos los agentes en ejecución se cancelan; las intenciones que queden en estado running tras reiniciar se restablecen a open y se vuelven a ejecutar",
		"ja": "バックエンドプロセスが SIGINT または SIGTERM を受け取り、再起動・更新・終了処理中です。実行中のすべてのエージェントはキャンセルされ、再起動後に残った running 状態のインテントは open にリセットされて再実行されます",
	},
	"单次运行的硬超时兜底已触发": {
		"en": "single-run hard timeout backstop triggered",
		"ko": "단일 실행 하드 타임아웃 안전장치가 발동했습니다",
		"es": "se activó el límite estricto de tiempo de espera de una ejecución",
		"ja": "単一実行のハードタイムアウト上限が発動しました",
	},
	"单次运行超过软墙钟预算及额外宽限，说明模型请求或某个工具长时间没有返回，导致正常的回合边界收尾无法执行。请重点检查中断前最后一个未返回的工具调用": {
		"en": "This run exceeded the soft wall-clock budget plus grace margin, indicating a model request or tool call hung for too long so normal turn-boundary wrap-up could not run. Check the last unreturned tool call before interruption",
		"ko": "단일 실행이 소프트 시간 예산과 추가 유예 시간을 초과했습니다. 모델 요청이나 도구가 오래 응답하지 않아 정상적인 턴 경계 마무리가 실행되지 못한 것이므로, 중단 직전 응답하지 않은 마지막 도구 호출을 확인하십시오",
		"es": "Esta ejecución superó el presupuesto de tiempo real blando más el margen adicional, lo que indica que una solicitud al modelo o una herramienta tardó demasiado y no pudo ejecutarse el cierre normal del turno. Revise la última llamada a herramienta sin respuesta antes de la interrupción",
		"ja": "単一実行がソフト実時間予算と追加猶予を超過しました。モデルリクエストまたはツールが長時間応答せず、通常のターン境界での終了処理を実行できませんでした。中断直前に応答が返らなかった最後のツール呼び出しを確認してください",
	},
	"上游 context 到达 deadline": {
		"en": "upstream context reached deadline",
		"ko": "상위 context 가 deadline 에 도달했습니다",
		"es": "el context superior alcanzó su deadline",
		"ja": "上位 context が deadline に達しました",
	},
	"上游 context 到达 deadline，但设置方没有通过 WithTimeoutCause 附加具名原因: ": {
		"en": "An upstream context reached its deadline without attaching a named cause via WithTimeoutCause: ",
		"ko": "상위 context 가 deadline 에 도달했으나, 설정한 쪽에서 WithTimeoutCause 로 명명된 사유를 붙이지 않았습니다: ",
		"es": "Un context superior alcanzó su deadline sin adjuntar una causa con nombre mediante WithTimeoutCause: ",
		"ja": "上位 context が deadline に達しましたが、設定側が WithTimeoutCause で名前付きの原因を付与していません: ",
	},
	"取消方未附加具名原因": {
		"en": "canceller attached no named cause",
		"ko": "취소한 쪽이 명명된 사유를 붙이지 않았습니다",
		"es": "quien canceló no adjuntó ninguna causa con nombre",
		"ja": "キャンセル側が名前付きの原因を付与していません",
	},
	"上游 context 被取消，但取消方没有通过 context.WithCancelCause 附加具名原因；请在 agent/cancelcause.go 登记原因并接入该取消点": {
		"en": "An upstream context was cancelled without attaching a named cause via context.WithCancelCause; register a cause in agent/cancelcause.go and wire it at that cancellation site",
		"ko": "상위 context 가 취소되었으나 취소한 쪽에서 context.WithCancelCause 로 명명된 사유를 붙이지 않았습니다. agent/cancelcause.go 에 사유를 등록하고 해당 취소 지점에 연결하십시오",
		"es": "Se canceló un context superior sin adjuntar una causa con nombre mediante context.WithCancelCause; registre una causa en agent/cancelcause.go y conéctela en ese punto de cancelación",
		"ja": "上位 context がキャンセルされましたが、キャンセル側が context.WithCancelCause で名前付きの原因を付与していません。agent/cancelcause.go に原因を登録し、該当箇所へ接続してください",
	},
	"任务等待运行准入": {
		"en": "task waiting for run admission",
		"ko": "작업이 실행 승인을 기다리는 중입니다",
		"es": "la tarea espera admisión para ejecutarse",
		"ja": "タスクは実行許可待ちです",
	},
	"任务正在等待并发队列或准入状态提交，本次执行已停止；只有获得运行槽后才会重新领取意图": {
		"en": "The task is waiting on the concurrency queue or admission commit, so this execution stopped; intents will only be claimed again once a run slot is granted",
		"ko": "작업이 동시 실행 대기열 또는 승인 상태 반영을 기다리고 있어 이번 실행이 중지되었습니다. 실행 슬롯을 확보한 뒤에만 의도를 다시 할당받습니다",
		"es": "La tarea está esperando en la cola de concurrencia o la confirmación de admisión, por lo que esta ejecución se detuvo; solo volverá a reclamar intenciones cuando obtenga un turno",
		"ja": "タスクは同時実行キューまたは許可状態の確定を待っているため今回の実行を停止しました。実行スロットを獲得した後にのみインテントを再取得します",
	},
	"LLM 不可用，任务进入等待队列": {
		"en": "LLM unavailable; task entered waiting queue",
		"ko": "LLM 을 사용할 수 없어 작업이 대기열에 등록되었습니다",
		"es": "LLM no disponible; la tarea pasó a la cola de espera",
		"ja": "LLM が利用できないためタスクは待機キューに入りました",
	},
	"任务当前无法解析可运行的 Planner/Worker LLM，已释放并发槽；配置恢复后按队列顺序继续": {
		"en": "The task cannot currently resolve a usable Planner/Worker LLM and released its concurrency slot; it will continue in queue order once a configuration recovers",
		"ko": "작업이 현재 실행 가능한 Planner/Worker LLM 을 해석할 수 없어 동시 실행 슬롯을 반납했습니다. 설정이 복구되면 대기열 순서대로 재개됩니다",
		"es": "La tarea no puede resolver actualmente un LLM ejecutable para Planner/Worker y liberó su espacio de concurrencia; continuará en orden de cola cuando se recupere una configuración",
		"ja": "タスクは現在利用可能な Planner/Worker 用 LLM を解決できないため同時実行スロットを解放しました。設定が復旧するとキュー順に再開します",
	},
}

// displayMsg translates one source fragment. A miss (or "zh") returns the source.
func displayMsg(src string) string {
	if m, ok := terminalDisplayCatalog[src]; ok {
		if v, ok2 := m[config.ActiveLanguage()]; ok2 && v != "" {
			return v
		}
	}
	return src
}

// displayMsgPrefix translates a possibly TRUNCATED source fragment. Summaries are
// built with firstLine(hint, 80), so the reason hint embedded in them can be cut
// off mid-sentence and will never match a catalog key exactly. This returns the
// translation of the longest catalog key that begins with the given text; a
// fragment that matches nothing is returned untouched.
func displayMsgPrefix(src string) string {
	if src == "" {
		return src
	}
	if out := displayMsg(src); out != src {
		return out
	}
	trimmed := strings.TrimSuffix(strings.TrimSuffix(src, "..."), "…")
	if trimmed == "" {
		return src
	}
	best := ""
	for key := range terminalDisplayCatalog {
		if strings.HasPrefix(key, trimmed) && len(key) > len(best) {
			best = key
		}
	}
	if best == "" {
		return src
	}
	return displayMsg(best)
}

// DisplaySummary translates a terminal/capture summary for display. The stored
// row keeps the original; only the API's summary field goes through here.
func DisplaySummary(s string) string {
	if s == "" || config.ActiveLanguage() == "zh" {
		return s
	}
	for _, r := range terminalSummaryRules {
		m := r.pattern.FindStringSubmatch(s)
		if m == nil {
			continue
		}
		if out, ok := r.render(m); ok {
			return out
		}
	}
	// Plain capture-time prefix, no template.
	if strings.HasPrefix(s, "执行出错: ") {
		return displayMsg("执行出错: ") + strings.TrimPrefix(s, "执行出错: ")
	}
	return displayMsg(s)
}

// DisplayDetail translates a terminal detail block for display. Only the framing
// lines are rewritten; code fences, JSON payloads, tool output and partial model
// text pass through untouched because no rule matches them.
func DisplayDetail(s string) string {
	if s == "" || config.ActiveLanguage() == "zh" {
		return s
	}
	if !strings.Contains(s, "- **终态**") && !strings.Contains(s, "（无文字总结") && !strings.Contains(s, "（运行被中断") {
		return s // not a terminal block; leave tool output / model text alone
	}
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = displayDetailLine(line)
	}
	return strings.Join(lines, "\n")
}

// displayDetailLine rewrites one bullet if it matches a known shape.
func displayDetailLine(line string) string {
	if strings.HasPrefix(line, "（运行被中断：") || strings.HasPrefix(line, "（达到运行预算上限(") || strings.HasPrefix(line, "（无文字总结，终态 ") {
		return DisplaySummary(line)
	}
	// "- **终态**: `model_error` - <hint>"
	if m := reTerminalState.FindStringSubmatch(line); m != nil {
		return displayMsg("- **终态**: ") + "`" + m[1] + "` - " + displayMsg(m[2])
	} // "- **中断原因** (`code`): <why>"
	if m := reInterruptCause.FindStringSubmatch(line); m != nil {
		return displayMsg("- **中断原因** (`") + m[1] + displayMsg("`): ") + displayAbortWhy(m[2])
	}
	// "- **中断原因**: ..." (no named cause)
	if strings.HasPrefix(line, "- **中断原因**: ") {
		return displayMsg("- **中断原因**: 无法取得；取消方可能没有通过 context.WithCancelCause 附加具名原因")
	}
	// "- **底层错误**: `<err>`"
	if m := reUnderlyingErr.FindStringSubmatch(line); m != nil {
		return displayMsg("- **底层错误**: `") + m[1] + "`"
	}
	if strings.HasPrefix(line, "- **取消前已生成的部分输出**") {
		return displayMsg("- **取消前已生成的部分输出**:")
	}
	// "- **已执行**: N 轮模型回合"
	if m := reExecutedTurns.FindStringSubmatch(line); m != nil {
		return displayMsg("- **已执行**: ") + m[1] + displayMsg(" 轮模型回合")
	}
	// "- **本次运行耗时**: <dur>"
	if m := reRunDuration.FindStringSubmatch(line); m != nil {
		return displayMsg("- **本次运行耗时**: ") + m[1]
	}
	// "- **累计 token**: 输入 a / 输出 b / 缓存读 c / 缓存写 d"
	if m := reCumulativeTokens.FindStringSubmatch(line); m != nil {
		return displayMsg("- **累计 token**: 输入 ") + m[1] +
			displayMsg(" / 输出 ") + m[2] +
			displayMsg(" / 缓存读 ") + m[3] +
			displayMsg(" / 缓存写 ") + m[4]
	}
	if strings.HasPrefix(line, "- **工具调用**: ") {
		return displayMsg("- **工具调用**: 本次运行还没有发出工具调用就结束了")
	}
	// "- **中断时正在执行的工具**: `x`（已运行 1.2s，**未返回结果**）"
	if m := rePendingTool.FindStringSubmatch(line); m != nil {
		return displayMsg("- **中断时正在执行的工具**: `") + m[1] + "`" +
			displayMsg("（已运行 ") + m[2] + displayMsg("，**未返回结果**）")
	}
	// "- **中断前最后一个工具**: `x`（已正常返回）"
	if m := reLastTool.FindStringSubmatch(line); m != nil {
		return displayMsg("- **中断前最后一个工具**: `") + m[1] + "`" + displayMsg("（已正常返回）")
	}
	return line
}

func displayAbortWhy(why string) string {
	const deadlinePrefix = "上游 context 到达 deadline，但设置方没有通过 WithTimeoutCause 附加具名原因: "
	if strings.HasPrefix(why, deadlinePrefix) {
		return displayMsg(deadlinePrefix) + strings.TrimPrefix(why, deadlinePrefix)
	}
	return displayMsg(why)
}

// terminalSummaryRule rewrites one summary framing shape.
type terminalSummaryRule struct {
	pattern *regexp.Regexp
	render  func(m []string) (string, bool)
}

var terminalSummaryRules = []terminalSummaryRule{
	// "（运行被中断：<short>；停在<stage><suffix>，未完成）"
	{pattern: regexp.MustCompile(`^（运行被中断：(.*?)；停在(.*?)(，已运行 .*?)?，未完成）$`), render: func(m []string) (string, bool) {
		out := displayMsg("（运行被中断：") + displayMsg(m[1]) +
			displayMsg("；停在") + displayMsg(m[2]) + displayProgressSuffix(m[3]) +
			displayMsg("，未完成）")
		return out, true
	}},
	// "（达到运行预算上限(<reason>)，已收尾写回事实<suffix>；本次无文字总结）"
	{pattern: regexp.MustCompile(`^（达到运行预算上限\((.*?)\)，已收尾写回事实(，已运行 .*?)?；本次无文字总结）$`), render: func(m []string) (string, bool) {
		out := displayMsg("（达到运行预算上限(") + m[1] +
			displayMsg(")，已收尾写回事实") + displayProgressSuffix(m[2]) +
			displayMsg("；本次无文字总结）")
		return out, true
	}},
	// "（无文字总结，终态 <label>：<hint>）"
	{pattern: regexp.MustCompile(`^（无文字总结，终态 (.*?)：(.*?)）$`), render: func(m []string) (string, bool) {
		out := displayMsg("（无文字总结，终态 ") + m[1] + displayMsg("：") + displayMsgPrefix(m[2]) + displayMsg("）")
		return out, true
	}},
}

// displayProgressSuffix translates the "，已运行 <n> 轮 / <dur>" suffix when present.
func displayProgressSuffix(s string) string {
	if s == "" {
		return ""
	}
	// Strip the leading fragment marker, then translate the joined parts.
	body := strings.TrimPrefix(s, "，已运行 ")
	parts := strings.Split(body, " / ")
	for i, p := range parts {
		if strings.HasSuffix(p, " 轮") {
			parts[i] = strings.TrimSuffix(p, " 轮") + displayMsg(" 轮")
		}
	}
	return displayMsg("，已运行 ") + strings.Join(parts, " / ")
}

var (
	reTerminalState    = regexp.MustCompile("^- \\*\\*终态\\*\\*: `([^`]+)` - (.*)$")
	reInterruptCause   = regexp.MustCompile("^- \\*\\*中断原因\\*\\* \\(`([^`]+)`\\): (.*)$")
	reUnderlyingErr    = regexp.MustCompile("^- \\*\\*底层错误\\*\\*: `(.*)`$")
	reExecutedTurns    = regexp.MustCompile("^- \\*\\*已执行\\*\\*: (\\d+) 轮模型回合$")
	reRunDuration      = regexp.MustCompile("^- \\*\\*本次运行耗时\\*\\*: (.*)$")
	reCumulativeTokens = regexp.MustCompile("^- \\*\\*累计 token\\*\\*: 输入 (\\d+) / 输出 (\\d+) / 缓存读 (\\d+) / 缓存写 (\\d+)$")
	rePendingTool      = regexp.MustCompile("^- \\*\\*中断时正在执行的工具\\*\\*: `([^`]+)`（已运行 (.*?)，\\*\\*未返回结果\\*\\*）$")
	reLastTool         = regexp.MustCompile("^- \\*\\*中断前最后一个工具\\*\\*: `([^`]+)`（已正常返回）$")
)
