package server

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/Autumn-27/artex/agent"
	"github.com/Autumn-27/artex/config"
	"github.com/Autumn-27/artex/sidequestion"
)

// activityMsgCatalog maps a server-generated activity summary (the exact Korean
// source literal) to its en/zh/es/ja translations. It is applied ONLY at the API
// boundary (activityDTO / activityDTOs), never when the row is written, because
// these rows are persisted history:
//
//   - Storage stays language-neutral. A summary written while the UI was Korean
//     must render in whatever language the user picks later, and switching back
//     and forth must not rewrite history.
//   - Some summaries are also agent input (see the "brain boundary" notes on each
//     constant). Translating at write time would leak a display decision into the
//     model's context; translating at read time cannot, because the brain reads
//     the stored row directly from the store, not through this DTO layer.
//
// Keys are byte-exact source literals (format verbs included: "%d차 계획 수립"),
// so a lookup miss means "no translation yet" and the Korean original renders
// unchanged — never a crash and never a garbled format string. Translations are
// therefore applied to the TEMPLATE before fmt fills it, and
// TestActivityMsgCatalogFormatVerbsMatch pins that every translation keeps the
// source's verbs in the same order.
//
// Why this lives beside errmsg_catalog.go rather than in config/i18n.go: the
// summaries are produced by the server package (engine/goals/task_llm/…), so only
// server can see those literals, and only server needs to translate them (the
// DTO layer). Lower-layer packages (db, notify, agent) that must translate their
// own messages use config.TrMsg instead.
//
// MAINTENANCE: upstream-sync drift. If an upstream change alters one of these
// source strings, the key stops matching and the new Korean/Chinese text renders
// verbatim (fail-open, by design). TestActivitySummariesTranslated names the
// exact missing string, so re-adding a key is a one-line fix — no retranslation
// of the whole catalog, and no silent breakage. See MAINTAINING.md §"번역 대칭과
// 드리프트 검사".
var activityMsgCatalog = map[string]map[string]string{
	// server/goals.go — round-0 goal breakdown marker (planner lane, no node_id).
	"0차 목표 분해": {"en": "Round-0 goal breakdown", "zh": "第 0 轮目标分解", "es": "Desglose de objetivos de la ronda 0", "ja": "第0ラウンド目標分解"},
	// server/engine.go — planner round boundary marker.
	"%d차 계획 수립": {"en": "Planning round %d", "zh": "第 %d 轮规划", "es": "Ronda de planificación %d", "ja": "第%dラウンド計画策定"},
	// server/engine.go — goalless (manually injected intents) task finished.
	"모든 목표를 달성했고 직접 투입한 의도의 실행도 끝나, 작업을 종료합니다": {"en": "All goals were met and the manually injected intents finished running; the task is now closed", "zh": "已达成所有目标，且直接投入的意图也已执行完毕，任务结束", "es": "Se cumplieron todos los objetivos y las intenciones inyectadas manualmente terminaron de ejecutarse; la tarea finaliza", "ja": "すべての目標を達成し、直接投入したインテントの実行も完了したため、タスクを終了します"},
	// server/engine_timeout.go — final planning round forced by the task deadline.
	"작업 시간 초과 마무리·최종 판정(%d차)": {"en": "Task timeout wrap-up and final verdict (round %d)", "zh": "任务超时收尾与最终判定（第 %d 轮）", "es": "Cierre por tiempo de espera agotado y veredicto final (ronda %d)", "ja": "タスクタイムアウトの締めくくり・最終判定（第%dラウンド）"},
	// server/goals.go — concurrency-queue admission notices (system lane).
	"대기열 등록: 동시 실행 상한 %d개에 도달해, 빈자리가 나면 자동으로 시작합니다":     {"en": "Queued: the concurrency limit of %d is reached; it starts automatically when a slot frees up", "zh": "已加入队列：并发上限 %d 个已满，有空位时将自动开始", "es": "En cola: se alcanzó el límite de concurrencia de %d; se iniciará automáticamente cuando quede un hueco", "ja": "キューに登録: 同時実行上限 %d に達したため、空きが出ると自動的に開始します"},
	"대기열 등록: 현재 실행 가능한 LLM 설정이 없어, 설정이 복구되면 자동으로 시작합니다": {"en": "Queued: no usable LLM configuration right now; it starts automatically once a configuration is restored", "zh": "已加入队列：当前没有可用的 LLM 配置，配置恢复后将自动开始", "es": "En cola: no hay una configuración de LLM utilizable ahora mismo; se iniciará automáticamente cuando se restablezca una configuración", "ja": "キューに登録: 現在利用できる LLM 設定がないため、設定が復旧すると自動的に開始します"},
	"대기열 등록: 먼저 대기 중인 작업이 있어, FIFO 순서대로 자동으로 시작합니다":     {"en": "Queued: another task is already waiting; it starts automatically in FIFO order", "zh": "已加入队列：已有更早等待的任务，将按 FIFO 顺序自动开始", "es": "En cola: ya hay otra tarea esperando; se iniciará automáticamente por orden FIFO", "ja": "キューに登録: 先に待機中のタスクがあるため、FIFO の順序で自動的に開始します"},

	// server/task_llm.go — automatic LLM failover after quota exhaustion.
	"%s 额度不足":     {"en": "Out of quota for %s", "zh": "%s 额度不足", "es": "Sin cuota para %s", "ja": "%s の割り当てを使い切りました"},
	"，后续调用切换到 %s": {"en": "; later calls switch to %s", "zh": "，后续调用切换到 %s", "es": "; las llamadas siguientes pasan a %s", "ja": "。以降の呼び出しは %s に切り替えます"},
	"，配置链已耗尽":     {"en": "; the configuration chain is exhausted", "zh": "，配置链已耗尽", "es": "; la cadena de configuración está agotada", "ja": "。設定チェーンを使い切りました"},
	// server/task_llm.go — manual task LLM switch.
	"用户手动将任务 LLM 从 %s 切换到 %s": {"en": "The task LLM was switched manually from %s to %s", "zh": "用户手动将任务 LLM 从 %s 切换到 %s", "es": "El LLM de la tarea se cambió manualmente de %s a %s", "ja": "タスクの LLM を %s から %s に手動で切り替えました"},
	// server/task_llm.go — profile label fallbacks.
	"默认配置":   {"en": "Default configuration", "zh": "默认配置", "es": "Configuración predeterminada", "ja": "既定の設定"},
	"配置 #%d": {"en": "Configuration #%d", "zh": "配置 #%d", "es": "Configuración n.º %d", "ja": "設定 #%d"},
	"%s（%s）": {"en": "%s (%s)", "zh": "%s（%s）", "es": "%s (%s)", "ja": "%s（%s）"},

	// server/task_archives.go — partial task restore.
	"작업 복원을 완료했지만 일부 연관 항목이 온전히 복원되지 못했습니다": {"en": "The task was restored, but some related items could not be fully restored", "zh": "任务已恢复，但部分关联项未能完整恢复", "es": "La tarea se restauró, pero algunos elementos relacionados no pudieron restaurarse por completo", "ja": "タスクは復元しましたが、一部の関連項目を完全には復元できませんでした"},

	// server/conversations.go — transcriptErrorSummary("메인 에이전트", …) label.
	"메인 에이전트": {"en": "Main agent", "zh": "主智能体", "es": "Agente principal", "ja": "メインエージェント"},
	// server/conversations.go — transcriptErrorSummary framing "(<label> 오류: <err>)".
	"오류: ": {"en": "error: ", "zh": "错误：", "es": "error: ", "ja": "エラー: "},

	// server/assets.go — task-asset provenance summaries (shown on the test-assets tab).
	"자산 API 로 등록":      {"en": "Registered via the asset API", "zh": "通过资产 API 注册", "es": "Registrado mediante la API de activos", "ja": "アセット API で登録"},
	"작업 설명 또는 목표로 초기화": {"en": "Initialized from the task description or goal", "zh": "由任务描述或目标初始化", "es": "Inicializado desde la descripción o el objetivo de la tarea", "ja": "タスクの説明または目標から初期化"},

	// server/findings_groups.go — follow-up exploit intent recorded on the timeline.
	"사용자가 제출한 취약점 심화 익스플로잇 의도": {"en": "A follow-up exploit intent submitted by the user", "zh": "用户提交的漏洞深入利用意图", "es": "Una intención de explotación de seguimiento enviada por el usuario", "ja": "ユーザーが投入した脆弱性深掘りエクスプロイトのインテント"},

	// server/notify_api.go — notification channel test delivery summary.
	"ARTEX 알림 채널 테스트 메시지입니다. 이 메시지를 받으셨다면 채널 설정이 정상입니다.": {"en": "This is an ARTEX notification channel test message. If you received it, the channel configuration is working", "zh": "这是 ARTEX 通知渠道测试消息。如果您收到了它，说明渠道配置正常", "es": "Este es un mensaje de prueba del canal de notificaciones de ARTEX. Si lo recibió, la configuración del canal funciona", "ja": "ARTEX 通知チャネルのテストメッセージです。これを受け取っていれば、チャネル設定は正常です"},

	// server/server.go — rule-mode chat fallback status summary and its aliases.
	"(규칙 모드, LLM 미설정) 현재 현황: 자산 %d개, 대기 의도 %d개, 확인된 취약점 %d개.\n사용 가능한 명령: \"의도 ...\"로 의도를 주입하고, \"힌트 ...\"로 플래너에게 힌트를 전달합니다.": {"en": "(rule mode, LLM not configured) Current status: %d assets, %d pending intents, %d confirmed vulnerabilities.\nAvailable commands: inject an intent with \"intent ...\", and pass a hint to the planner with \"hint ...\".", "zh": "（规则模式，未配置 LLM）当前状态：资产 %d 个、待处理意图 %d 个、已确认漏洞 %d 个。\n可用命令：使用 \"意图 ...\" 注入意图，使用 \"提示 ...\" 向 planner 传递提示。", "es": "(modo de reglas, LLM no configurado) Estado actual: %d activos, %d intenciones pendientes, %d vulnerabilidades confirmadas.\nComandos disponibles: inyecte una intención con \"intención ...\" y envíe una pista al planner con \"pista ...\".", "ja": "(ルールモード、LLM 未設定) 現在の状況: 資産 %d 件、待機中のインテント %d 件、確認済みの脆弱性 %d 件。\n利用可能なコマンド: \"インテント ...\" でインテントを投入し、\"ヒント ...\" でプランナーにヒントを渡します。"},
	"의도": {"en": "intent", "zh": "意图", "es": "intención", "ja": "インテント"},
	"힌트": {"en": "hint", "zh": "提示", "es": "pista", "ja": "ヒント"},

	// intercept/intercept.go — human-approval timeline card and decision reasons.
	"도구 %s 승인 요청 (#%d)":                      {"en": "Tool %s approval request (#%d)", "zh": "工具 %s 审批请求 (#%d)", "es": "Solicitud de aprobación para la herramienta %s (#%d)", "ja": "ツール %s の承認リクエスト (#%d)"},
	"사람이 실행을 허용했습니다":                         {"en": "A human allowed the execution", "zh": "人工已允许执行", "es": "Una persona permitió la ejecución", "ja": "人が実行を許可しました"},
	"사람이 실행을 거부했습니다":                         {"en": "A human denied the execution", "zh": "人工已拒绝执行", "es": "Una persona rechazó la ejecución", "ja": "人が実行を拒否しました"},
	"작업이 취소되었습니다":                            {"en": "The task was cancelled", "zh": "任务已取消", "es": "La tarea se canceló", "ja": "タスクはキャンセルされました"},
	"실행 전에 작업이 취소되었습니다":                      {"en": "The task was cancelled before execution", "zh": "执行前任务已取消", "es": "La tarea se canceló antes de la ejecución", "ja": "実行前にタスクがキャンセルされました"},
	"승인 시간이 초과되어 시간 초과 정책에 따라 처리합니다":         {"en": "The approval timed out; the timeout policy is applied", "zh": "审批超时，按超时策略处理", "es": "La aprobación agotó el tiempo de espera; se aplica la política de tiempo de espera", "ja": "承認がタイムアウトしたため、タイムアウトポリシーに従って処理します"},
	"검토 맥락이 완전하지 않아 사람의 확인이 필요합니다: ":         {"en": "The review context is incomplete, so human confirmation is required: ", "zh": "审查上下文不完整，需要人工确认：", "es": "El contexto de revisión está incompleto, por lo que se requiere confirmación humana: ", "ja": "レビューの文脈が不完全なため、人による確認が必要です: "},
	"모델 승인에 실패하여 실패 정책에 따라 처리합니다: ":          {"en": "Model approval failed; the failure policy is applied: ", "zh": "模型审批失败，按失败策略处理：", "es": "La aprobación del modelo falló; se aplica la política de fallo: ", "ja": "モデル承認に失敗したため、失敗ポリシーに従って処理します: "},
	"모델 출력을 해석할 수 없어 실패 정책에 따라 처리합니다":        {"en": "The model output could not be parsed; the failure policy is applied", "zh": "无法解析模型输出，按失败策略处理", "es": "No se pudo interpretar la salida del modelo; se aplica la política de fallo", "ja": "モデル出力を解釈できないため、失敗ポリシーに従って処理します"},
	"가로채기 규칙 [":                              {"en": "Interception rule [", "zh": "拦截规则 [", "es": "Regla de interceptación [", "ja": "インターセプトルール ["},
	"] 에 따라 이 도구를 실행할 수 없습니다":                {"en": "] forbids running this tool", "zh": "] 禁止执行此工具", "es": "] impide ejecutar esta herramienta", "ja": "] により、このツールは実行できません"},
	"] 에 따라 사용자 승인이 필요합니다. 잠시 기다려 주세요":       {"en": "] requires user approval. Please wait", "zh": "] 需要用户审批，请稍候", "es": "] requiere aprobación del usuario. Espere", "ja": "] のためユーザーの承認が必要です。しばらくお待ちください"},
	"이미 처리되었거나 존재하지 않는 승인입니다. 기록을 새로 고쳐 주세요": {"en": "This approval was already resolved or does not exist. Refresh the record and try again", "zh": "该审批已处理或不存在，请刷新记录后重试", "es": "Esta aprobación ya se resolvió o no existe. Actualice el registro e inténtelo de nuevo", "ja": "この承認は既に処理済みか存在しません。記録を更新してから再試行してください"},

	// intercept/intercept.go — rule-read failure falls safe to a human.
	"拦截规则读取失败，按失败安全策略转人工确认：": {"en": "Reading the interception rules failed; falling back to human confirmation (fail-safe): ", "zh": "拦截规则读取失败，按失败安全策略转人工确认：", "es": "No se pudieron leer las reglas de interceptación; se pasa a confirmación humana (a prueba de fallos): ", "ja": "インターセプトルールの読み取りに失敗したため、フェイルセーフ方針で人による確認に切り替えます: "},

	// db/finding_retests.go — retest interruption notices surfaced in the retest panel.
	"서비스가 재시작되어 재검증이 중단되었습니다. 다시 시작해 주세요":             {"en": "The service restarted, so the retest was interrupted. Start it again", "zh": "服务已重启，复测被中断。请重新开始", "es": "El servicio se reinició, por lo que la re-prueba se interrumpió. Iníciela de nuevo", "ja": "サービスが再起動したため再検証が中断されました。もう一度開始してください"},
	"에이전트가 재검증 결론을 저장하지 않았습니다. 대화를 확인한 뒤 다시 재검증해 주세요": {"en": "The agent did not save a retest conclusion. Check the conversation, then run the retest again", "zh": "智能体未保存复测结论。请检查对话后重新复测", "es": "El agente no guardó una conclusión de la re-prueba. Revise la conversación y vuelva a ejecutar la re-prueba", "ja": "エージェントが再検証の結論を保存していません。会話を確認してから再検証してください"},
	"사용자가 테스트 자산 페이지에서 직접 추가":                         {"en": "Added manually by the user on the test-assets page", "zh": "用户在测试资产页面手动添加", "es": "Añadido manualmente por el usuario en la página de activos de prueba", "ja": "ユーザーがテストアセットページで手動で追加"},

	// server/server.go — seedFirstIntent summary framing (persisted in Chinese for planner).
	"完成任务目标：": {"en": "Complete task goal: ", "ko": "작업 목표 달성: ", "es": "Completar objetivo de la tarea: ", "ja": "タスク目標を達成: "},
	"（任务：":    {"en": " (Task: ", "ko": " (작업: ", "es": " (Tarea: ", "ja": "（タスク: "},
	"）":      {"en": ")", "ko": ")", "es": ")", "ja": "）"},

	// db/task_assets.go, db/tasks.go, db/schema.sql — task-asset provenance & scope summaries.
	"의도가 앵커로 연결한 자산":  {"en": "Asset anchored by an intent", "zh": "由意图通过锚点关联的资产", "es": "Activo anclado por una intención", "ja": "インテントがアンカーで関連付けた資産"},
	"작업 생성 시 연결된 회사":   {"en": "Company linked at task creation", "zh": "创建任务时关联的企业", "es": "Empresa vinculada al crear la tarea", "ja": "タスク作成時に関連付けられた企業"},
	"작업 생성 시 연결된 회사: ": {"en": "Company linked at task creation: ", "zh": "创建任务时关联的企业：", "es": "Empresa vinculada al crear la tarea: ", "ja": "タスク作成時に関連付けられた企業: "},
	"任务执行期间自动关联":      {"en": "Automatically linked during task execution", "ko": "작업 실행 중 자동 연결됨", "es": "Vinculado automáticamente durante la ejecución de la tarea", "ja": "タスク実行中に自動関連付け"},
	"由历史任务资产关联迁移":     {"en": "Migrated from legacy task-asset link", "ko": "이전 작업-자산 연결에서 마이그레이션됨", "es": "Migrado de vínculos heredados entre tarea y activo", "ja": "従来のタスク・資産関連付けから移行"},

	// server/conversations.go — finding retest failure reasons, default titles & trigger titles.
	"재검증을 시작하지 못했습니다":              {"en": "Failed to start the retest", "zh": "无法启动复测", "es": "No se pudo iniciar la re-prueba", "ja": "再検証を開始できませんでした"},
	"재검증 상태를 읽지 못했습니다. 다시 시작해 주세요": {"en": "Failed to read the retest status. Please start again", "zh": "无法读取复测状态，请重新开始", "es": "No se pudo leer el estado de la re-prueba. Iníciela de nuevo", "ja": "再検証の状態を読み取れませんでした。もう一度開始してください"},
	"재검증이 중지되었거나 서비스가 종료되었습니다":     {"en": "The retest was stopped or the service shut down", "zh": "复测已停止或服务已关闭", "es": "La re-prueba se detuvo o el servicio se cerró", "ja": "再検証が停止されたか、サービスが終了しました"},
	"请复测漏洞 #%d":                  {"en": "Please retest vulnerability #%d", "ko": "취약점 #%d 재검증을 수행해 주세요", "es": "Vuelva a probar la vulnerabilidad #%d", "ja": "脆弱性 #%d を再検証してください"},
	"새 대화":                        {"en": "New conversation", "zh": "新对话", "es": "Nueva conversación", "ja": "新しい会話"},
	"첨부 메시지":                      {"en": "Attachment message", "zh": "附件消息", "es": "Mensaje con adjuntos", "ja": "添付メッセージ"},
	"未分类":                         {"en": "Uncategorized", "ko": "미분류", "es": "Sin categoría", "ja": "未分類"},
	"复测 #%d · %s":                 {"en": "Retest #%d · %s", "ko": "재검증 #%d · %s", "es": "Re-prueba #%d · %s", "ja": "再検証 #%d · %s"},
	"合并触发 · task#%d · %d 条":       {"en": "Batched triggers · task#%d · %d", "ko": "병합 트리거 · task#%d · %d건", "es": "Disparadores agrupados · task#%d · %d", "ja": "統合トリガー · task#%d · %d件"},
	"合并触发 · 全部 · %d 条":            {"en": "Batched triggers · All · %d", "ko": "병합 트리거 · 전체 · %d건", "es": "Disparadores agrupados · Todo · %d", "ja": "統合トリガー · すべて · %d件"},
	"超时触发 · task#%d":              {"en": "Timeout trigger · task#%d", "ko": "시간 초과 트리거 · task#%d", "es": "Disparador por tiempo agotado · task#%d", "ja": "タイムアウトトリガー · task#%d"},
	"任务创建触发 · task#%d":            {"en": "Task-created trigger · task#%d", "ko": "작업 생성 트리거 · task#%d", "es": "Disparador al crear tarea · task#%d", "ja": "タスク作成トリガー · task#%d"},
	"工具触发 · %s · task#%d":         {"en": "Tool trigger · %s · task#%d", "ko": "도구 트리거 · %s · task#%d", "es": "Disparador de herramienta · %s · task#%d", "ja": "ツールトリガー · %s · task#%d"},

	// db/task_archives_restore.go — archive restore warnings surfaced via system activity detail.
	"任务分类 %d 已删除，已恢复为未分类":     {"en": "Task category %d was deleted; restored as uncategorized", "ko": "작업 분류 %d 이(가) 삭제되어 미분류로 복원되었습니다", "es": "La categoría de tarea %d fue eliminada; se restauró sin categoría", "ja": "タスク分類 %d は削除されていたため未分類として復元されました"},
	"任务企业 %d 已删除，企业关联已跳过":     {"en": "Task company %d was deleted; company link skipped", "ko": "작업 기업 %d 이(가) 삭제되어 기업 연결을 건너뛰었습니다", "es": "La empresa %d de la tarea fue eliminada; se omitió el vínculo", "ja": "タスク企業 %d は削除されていたため企業の関連付けをスキップしました"},
	"LLM 配置 %d 已删除，已从任务配置中移除": {"en": "LLM configuration %d was deleted; removed from task configuration", "ko": "LLM 설정 %d 이(가) 삭제되어 작업 설정에서 제거되었습니다", "es": "La configuración de LLM %d fue eliminada; se quitó de la tarea", "ja": "LLM 設定 %d は削除されていたためタスク設定から除外されました"},
	"资产 %d 的企业 %d 已删除，已恢复为未归属": {"en": "Asset %d: company %d was deleted; restored as unassigned", "ko": "자산 %d 의 기업 %d 이(가) 삭제되어 미귀속으로 복원되었습니다", "es": "Activo %d: la empresa %d fue eliminada; se restauró sin asignar", "ja": "資産 %d の企業 %d は削除されていたため未帰属として復元されました"},
	"资产 ID %d 已被占用，恢复为 %d":    {"en": "Asset ID %d was already taken; restored as %d", "ko": "자산 ID %d 이(가) 이미 사용 중이어서 %d (으)로 복원되었습니다", "es": "El ID de activo %d ya estaba ocupado; se restauró como %d", "ja": "資産 ID %d は既に使用されていたため %d として復元されました"},
	"来源任务 %d 不可用，继承关系已跳过":     {"en": "Source task %d is unavailable; inheritance link skipped", "ko": "출처 작업 %d 을(를) 사용할 수 없어 상속 관계를 건너뛰었습니다", "es": "La tarea de origen %d no está disponible; se omitió la herencia", "ja": "元のタスク %d は利用できないため継承関係をスキップしました"},
	"企业 %d 已删除，关联范围已跳过":       {"en": "Company %d was deleted; linked scope skipped", "ko": "기업 %d 이(가) 삭제되어 연결 범위를 건너뛰었습니다", "es": "La empresa %d fue eliminada; se omitió el alcance vinculado", "ja": "企業 %d は削除されていたため関連範囲をスキップしました"},
	"LLM 配置 %d 已删除，配置链项已跳过":   {"en": "LLM configuration %d was deleted; configuration chain entry skipped", "ko": "LLM 설정 %d 이(가) 삭제되어 설정 체인 항목을 건너뛰었습니다", "es": "La configuración de LLM %d fue eliminada; se omitió la entrada de la cadena", "ja": "LLM 設定 %d は削除されていたため設定チェーン項目をスキップしました"},
}

// activityMsg translates one stored activity summary into the active output
// language. A miss (or "ko") returns the source unchanged, so untranslated rows
// render exactly as before. Callers must pass the TEMPLATE before fmt fills it.
func activityMsg(src string) string {
	if m, ok := activityMsgCatalog[src]; ok {
		if v, ok2 := m[config.ActiveLanguage()]; ok2 && v != "" {
			return v
		}
	}
	return src
}

// translateActivitySummary maps a persisted summary to the active display
// language. It handles both plain summaries (exact catalog lookup) and the
// templated summaries that were already formatted before storage:
//
//   - "%d차 계획 수립" → "Planning round 7": the stored row holds the FORMATTED
//     string, so an exact lookup misses. We re-parse it with the known template.
//   - "도구 Bash 승인 요청 (#42)" → "Tool Bash approval request (#42)".
//   - "가로채기 규칙 [R1] 에 따라 …" → "Interception rule [R1] forbids …": the
//     reason is assembled from two catalog fragments around the rule name.
//
// Anything that matches nothing is returned verbatim, so an unrecognized (or
// upstream-rewritten) summary degrades to "shown as stored", never to empty.
func translateActivitySummary(src string) string {
	if src == "" {
		return src
	}
	if out := activityMsg(src); out != src {
		return out
	}
	for _, t := range activitySummaryPatterns {
		m := t.pattern.FindStringSubmatch(src)
		if m == nil {
			continue
		}
		if out, ok := t.render(m); ok {
			return out
		}
	}
	return agent.DisplaySummary(src)
}

// translateSeedIntentSummary translates the Chinese framing around a seed
// intent's goal/description at the DTO boundary while keeping the stored node
// payload untouched for the planner.
func translateSeedIntentSummary(src string) string {
	if config.ActiveLanguage() == "zh" || !strings.HasPrefix(src, "完成任务目标：") {
		return src
	}
	prefix := activityMsg("完成任务目标：")
	rest := strings.TrimPrefix(src, "完成任务目标：")
	if idx := strings.LastIndex(rest, "（任务："); idx >= 0 && strings.HasSuffix(rest, "）") {
		goal := rest[:idx]
		desc := strings.TrimSuffix(rest[idx+len("（任务："):], "）")
		return prefix + goal + activityMsg("（任务：") + desc + activityMsg("）")
	}
	return prefix + rest
}

// activitySummaryPattern is one templated summary shape: a catalog template plus a
// pattern that recovers the args from the already-formatted stored string. The
// render func decides how the translated fragments are recombined, which is
// needed for messages the server assembles from more than one fragment (the
// interception-rule messages are prefix + rule name + suffix).
type activitySummaryPattern struct {
	pattern *regexp.Regexp
	render  func(m []string) (string, bool)
}

// activitySummaryPatterns covers every templated summary the server formats
// before storing. Order matters: the more specific shapes come first.
var activitySummaryPatterns = []activitySummaryPattern{
	// engine.go planner round boundary: "%d차 계획 수립".
	{pattern: regexp.MustCompile(`^(\d+)차 계획 수립$`), render: func(m []string) (string, bool) {
		return fillTemplate("%d차 계획 수립", mustInt(m[1]))
	}},
	// engine_timeout.go final round: "작업 시간 초과 마무리·최종 판정(%d차)".
	{pattern: regexp.MustCompile(`^작업 시간 초과 마무리·최종 판정\((\d+)차\)$`), render: func(m []string) (string, bool) {
		return fillTemplate("작업 시간 초과 마무리·최종 판정(%d차)", mustInt(m[1]))
	}},
	// goals.go concurrency-limit queue notice: "…동시 실행 상한 %d개에…".
	{pattern: regexp.MustCompile(`^대기열 등록: 동시 실행 상한 (\d+)개에 도달해, 빈자리가 나면 자동으로 시작합니다$`), render: func(m []string) (string, bool) {
		return fillTemplate("대기열 등록: 동시 실행 상한 %d개에 도달해, 빈자리가 나면 자동으로 시작합니다", mustInt(m[1]))
	}},
	// intercept/intercept.go approval card: "도구 %s 승인 요청 (#%d)".
	{pattern: regexp.MustCompile(`^도구 (.+?) 승인 요청 \(#(\d+)\)$`), render: func(m []string) (string, bool) {
		return fillTemplate("도구 %s 승인 요청 (#%d)", m[1], mustInt(m[2]))
	}},
	// intercept/intercept.go default deny message, assembled from two fragments:
	// "가로채기 규칙 [" + rule + "] 에 따라 이 도구를 실행할 수 없습니다".
	{pattern: regexp.MustCompile(`^가로채기 규칙 \[(.+?)\] 에 따라 이 도구를 실행할 수 없습니다$`), render: func(m []string) (string, bool) {
		return joinFragments(
			"가로채기 규칙 [", m[1], "] 에 따라 이 도구를 실행할 수 없습니다",
		)
	}},
	// intercept/intercept.go default ask message, same two-fragment shape.
	{pattern: regexp.MustCompile(`^가로채기 규칙 \[(.+?)\] 에 따라 사용자 승인이 필요합니다\. 잠시 기다려 주세요$`), render: func(m []string) (string, bool) {
		return joinFragments(
			"가로채기 규칙 [", m[1], "] 에 따라 사용자 승인이 필요합니다. 잠시 기다려 주세요",
		)
	}},
	// task_llm.go automatic LLM failover switch.
	{pattern: regexp.MustCompile(`^(.+?) 额度不足，后续调用切换到 (.+)$`), render: func(m []string) (string, bool) {
		first, ok := fillTemplate("%s 额度不足", translateProfileLabel(m[1]))
		second, ok2 := fillTemplate("，后续调用切换到 %s", translateProfileLabel(m[2]))
		if !ok || !ok2 {
			return "", false
		}
		return first + second, true
	}},
	// task_llm.go automatic LLM failover exhausted.
	{pattern: regexp.MustCompile(`^(.+?) 额度不足，配置链已耗尽$`), render: func(m []string) (string, bool) {
		first, ok := fillTemplate("%s 额度不足", translateProfileLabel(m[1]))
		second := activityMsg("，配置链已耗尽")
		if !ok || second == "，配置链已耗尽" {
			return "", false
		}
		return first + second, true
	}},
	// task_llm.go manual task LLM switch.
	{pattern: regexp.MustCompile(`^用户手动将任务 LLM 从 (.+?) 切换到 (.+)$`), render: func(m []string) (string, bool) {
		return fillTemplate("用户手动将任务 LLM 从 %s 切换到 %s", translateProfileLabel(m[1]), translateProfileLabel(m[2]))
	}},
	// server.go rule-mode chat status fallback.
	{pattern: regexp.MustCompile(`^\(규칙 모드, LLM 미설정\) 현재 현황: 자산 (\d+)개, 대기 의도 (\d+)개, 확인된 취약점 (\d+)개\.\n사용 가능한 명령: "의도 \.\.\."로 의도를 주입하고, "힌트 \.\.\."로 플래너에게 힌트를 전달합니다\.$`), render: func(m []string) (string, bool) {
		return fillTemplate(
			"(규칙 모드, LLM 미설정) 현재 현황: 자산 %d개, 대기 의도 %d개, 확인된 취약점 %d개.\n사용 가능한 명령: \"의도 ...\"로 의도를 주입하고, \"힌트 ...\"로 플래너에게 힌트를 전달합니다.",
			mustInt(m[1]), mustInt(m[2]), mustInt(m[3]),
		)
	}},
	// intercept/intercept.go persisted fallback prefixes with appended error strings.
	{pattern: regexp.MustCompile(`^검토 맥락이 완전하지 않아 사람의 확인이 필요합니다: ([\s\S]*)$`), render: func(m []string) (string, bool) {
		return translatePrefix("검토 맥락이 완전하지 않아 사람의 확인이 필요합니다: ", m[1])
	}},
	{pattern: regexp.MustCompile(`^모델 승인에 실패하여 실패 정책에 따라 처리합니다: ([\s\S]*)$`), render: func(m []string) (string, bool) {
		return translatePrefix("모델 승인에 실패하여 실패 정책에 따라 처리합니다: ", m[1])
	}},
	{pattern: regexp.MustCompile(`^拦截规则读取失败，按失败安全策略转人工确认：([\s\S]*)$`), render: func(m []string) (string, bool) {
		return translatePrefix("拦截规则读取失败，按失败安全策略转人工确认：", m[1])
	}},
	// conversations.go / server.go transcriptErrorSummary("(메인 에이전트 오류: <err>)").
	{pattern: regexp.MustCompile(`^\(메인 에이전트 오류: ([\s\S]*)\)$`), render: func(m []string) (string, bool) {
		if config.ActiveLanguage() == "ko" {
			return "", false
		}
		return "(" + activityMsg("메인 에이전트") + " " + activityMsg("오류: ") + m[1] + ")", true
	}},
	// conversations.go transcriptErrorSummary("(오류: <err>)").
	{pattern: regexp.MustCompile(`^\(오류: ([\s\S]*)\)$`), render: func(m []string) (string, bool) {
		if config.ActiveLanguage() == "ko" {
			return "", false
		}
		return "(" + activityMsg("오류: ") + m[1] + ")", true
	}},
	// db/tasks.go task_asset_links.source_summary with appended company name.
	{pattern: regexp.MustCompile(`^작업 생성 시 연결된 회사: ([\s\S]*)$`), render: func(m []string) (string, bool) {
		return translatePrefix("작업 생성 시 연결된 회사: ", m[1])
	}},
	// db/finding_retests.go initial user message summary "请复测漏洞 #%d".
	{pattern: regexp.MustCompile(`^请复测漏洞 #(\d+)$`), render: func(m []string) (string, bool) {
		return fillTemplate("请复测漏洞 #%d", mustInt(m[1]))
	}},
}

var (
	reProfileNumberLabel = regexp.MustCompile(`^配置 #(\d+)$`)
	reProfileWithDetail  = regexp.MustCompile(`^(.+?)（([^）]+)）$`)
)

func translateProfileLabel(s string) string {
	if config.ActiveLanguage() == "zh" {
		return s
	}
	if m := reProfileWithDetail.FindStringSubmatch(s); m != nil {
		name := translateProfileName(m[1])
		tmpl := activityMsg("%s（%s）")
		if tmpl != "%s（%s）" {
			return fmt.Sprintf(tmpl, name, m[2])
		}
		return name + "（" + m[2] + "）"
	}
	return translateProfileName(s)
}

func translateProfileName(name string) string {
	if name == "默认配置" {
		return activityMsg("默认配置")
	}
	if m := reProfileNumberLabel.FindStringSubmatch(name); m != nil {
		if out, ok := fillTemplate("配置 #%d", mustInt(m[1])); ok {
			return out
		}
	}
	return name
}

func mustInt(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}

func translatePrefix(prefix, rest string) (string, bool) {
	tr := activityMsg(prefix)
	if tr == prefix {
		return "", false
	}
	return tr + rest, true
}

// fillTemplate translates a catalog template and fills it with args. It reports
// false when the template has no translation, so the caller keeps the original.
func fillTemplate(template string, args ...any) (string, bool) {
	tmpl := activityMsg(template)
	if tmpl == template {
		return "", false
	}
	return fmt.Sprintf(tmpl, args...), true
}

// joinFragments translates a prefix, a literal middle value (a rule name — never
// translated), and a suffix, then concatenates them. It reports false if either
// fragment has no translation.
func joinFragments(prefix, middle, suffix string) (string, bool) {
	pre, ok := fillTemplate(prefix)
	if !ok {
		return "", false
	}
	suf, ok := fillTemplate(suffix)
	if !ok {
		return "", false
	}
	return pre + middle + suf, true
}

var (
	reConvRetestTitle        = regexp.MustCompile(`^复测 #(\d+) · ([\s\S]+)$`)
	reConvMergeTaskTitle     = regexp.MustCompile(`^合并触发 · task#(\d+) · (\d+) 条$`)
	reConvMergeAllTitle      = regexp.MustCompile(`^合并触发 · 全部 · (\d+) 条$`)
	reConvTimeoutTrigger     = regexp.MustCompile(`^超时触发 · task#(\d+)$`)
	reConvTaskCreateTrigger  = regexp.MustCompile(`^任务创建触发 · task#(\d+)$`)
	reConvToolCallTrigger    = regexp.MustCompile(`^工具触发 · (.+?) · task#(\d+)$`)
	archiveRestoreWarnRules = []activitySummaryPattern{
		{pattern: regexp.MustCompile(`^任务分类 (\d+) 已删除，已恢复为未分类$`), render: func(m []string) (string, bool) {
			return fillTemplate("任务分类 %d 已删除，已恢复为未分类", mustInt(m[1]))
		}},
		{pattern: regexp.MustCompile(`^任务企业 (\d+) 已删除，企业关联已跳过$`), render: func(m []string) (string, bool) {
			return fillTemplate("任务企业 %d 已删除，企业关联已跳过", mustInt(m[1]))
		}},
		{pattern: regexp.MustCompile(`^LLM 配置 (\d+) 已删除，已从任务配置中移除$`), render: func(m []string) (string, bool) {
			return fillTemplate("LLM 配置 %d 已删除，已从任务配置中移除", mustInt(m[1]))
		}},
		{pattern: regexp.MustCompile(`^资产 (\d+) 的企业 (\d+) 已删除，已恢复为未归属$`), render: func(m []string) (string, bool) {
			return fillTemplate("资产 %d 的企业 %d 已删除，已恢复为未归属", mustInt(m[1]), mustInt(m[2]))
		}},
		{pattern: regexp.MustCompile(`^资产 ID (\d+) 已被占用，恢复为 (\d+)$`), render: func(m []string) (string, bool) {
			return fillTemplate("资产 ID %d 已被占用，恢复为 %d", mustInt(m[1]), mustInt(m[2]))
		}},
		{pattern: regexp.MustCompile(`^来源任务 (\d+) 不可用，继承关系已跳过$`), render: func(m []string) (string, bool) {
			return fillTemplate("来源任务 %d 不可用，继承关系已跳过", mustInt(m[1]))
		}},
		{pattern: regexp.MustCompile(`^企业 (\d+) 已删除，关联范围已跳过$`), render: func(m []string) (string, bool) {
			return fillTemplate("企业 %d 已删除，关联范围已跳过", mustInt(m[1]))
		}},
		{pattern: regexp.MustCompile(`^LLM 配置 (\d+) 已删除，配置链项已跳过$`), render: func(m []string) (string, bool) {
			return fillTemplate("LLM 配置 %d 已删除，配置链项已跳过", mustInt(m[1]))
		}},
	}
)

// translateConversationTitle translates system-generated conversation titles at
// the API boundary without touching persisted rows or the "새 대화" sentinel.
func translateConversationTitle(title string) string {
	if title == "" {
		return title
	}
	if out := activityMsg(title); out != title {
		return out
	}
	if config.ActiveLanguage() == "zh" {
		return title
	}
	if m := reConvRetestTitle.FindStringSubmatch(title); m != nil {
		suffix := m[2]
		if suffix == "未分类" {
			suffix = activityMsg("未分类")
		}
		if out, ok := fillTemplate("复测 #%d · %s", mustInt(m[1]), suffix); ok {
			return out
		}
	}
	for _, r := range []activitySummaryPattern{
		{pattern: reConvMergeTaskTitle, render: func(m []string) (string, bool) {
			return fillTemplate("合并触发 · task#%d · %d 条", mustInt(m[1]), mustInt(m[2]))
		}},
		{pattern: reConvMergeAllTitle, render: func(m []string) (string, bool) {
			return fillTemplate("合并触发 · 全部 · %d 条", mustInt(m[1]))
		}},
		{pattern: reConvTimeoutTrigger, render: func(m []string) (string, bool) {
			return fillTemplate("超时触发 · task#%d", mustInt(m[1]))
		}},
		{pattern: reConvTaskCreateTrigger, render: func(m []string) (string, bool) {
			return fillTemplate("任务创建触发 · task#%d", mustInt(m[1]))
		}},
		{pattern: reConvToolCallTrigger, render: func(m []string) (string, bool) {
			return fillTemplate("工具触发 · %s · task#%d", m[1], mustInt(m[2]))
		}},
	} {
		if m := r.pattern.FindStringSubmatch(title); m != nil {
			if out, ok := r.render(m); ok {
				return out
			}
		}
	}
	return title
}

// translateArchiveWarnings translates persisted task-archive restore warnings
// line-by-line at read time while leaving unknown text untouched.
func translateArchiveWarnings(detail string) string {
	if detail == "" || config.ActiveLanguage() == "zh" {
		return detail
	}
	lines := strings.Split(detail, "\n")
	changed := false
	for i, line := range lines {
		for _, r := range archiveRestoreWarnRules {
			if m := r.pattern.FindStringSubmatch(line); m != nil {
				if out, ok := r.render(m); ok && out != line {
					lines[i] = out
					changed = true
				}
				break
			}
		}
	}
	if !changed {
		return detail
	}
	return strings.Join(lines, "\n")
}

// translateLazyActivityDetail translates lazy-loaded activity detail blobs for
// terminal blocks, archive restore warnings, and known server detail lines.
func translateLazyActivityDetail(detail string) string {
	if detail == "" {
		return detail
	}
	if out := agent.DisplayDetail(detail); out != detail {
		return out
	}
	if out := translateArchiveWarnings(detail); out != detail {
		return out
	}
	return translateActivitySummary(detail)
}

// translateSideQuestionError translates both Korean (server/sidequestion) and
// Chinese (db/side_questions) user-facing side-question errors at the API boundary.
func translateSideQuestionError(s string) string {
	if s == "" {
		return s
	}
	if out := trMsg(s); out != s {
		return out
	}
	const sideSummaryErrPrefix = "곁질문 요약에 실패했습니다: "
	if strings.HasPrefix(s, sideSummaryErrPrefix) {
		if trPre := trMsg(sideSummaryErrPrefix); trPre != sideSummaryErrPrefix {
			return trPre + strings.TrimPrefix(s, sideSummaryErrPrefix)
		}
	}
	return config.TrMsg(s)
}

// translateSideExchange returns a display copy of a side-question exchange with
// its error and tool-unavailable notice translated to the active output language.
func translateSideExchange(e sidequestion.Exchange) sidequestion.Exchange {
	e.Error = translateSideQuestionError(e.Error)
	const sideToolUnavailable = "현재 곁질문에서는 도구 작업을 실행할 수 없습니다. 작업 요청은 메인 대화에서 보내 주세요."
	if e.Answer == sideToolUnavailable {
		e.Answer = trMsg(sideToolUnavailable)
	}
	return e
}
