package config

// zhMsgCatalog maps a user-facing Chinese message (the exact source literal from
// the lower-layer packages) to its en/ko/es/ja translations. "zh" is the key
// itself and is not stored. See TrMsg for lookup.
//
// Why this catalog is separate from server/errmsg_catalog.go: that one is keyed by
// the Korean source strings the server package writes, and it lives in server
// because only server can see those literals. The messages cataloged here are
// produced by packages below server (db, agent, evidence, traffic). Those packages
// cannot import server (import cycle), but they can import config, so the catalog
// and its lookup must live here. The source language is Chinese, not Korean, so a
// single shared map would not work either: the keys differ.
//
// Format templates (%d/%s/%q/%w) must be translated at the call site BEFORE
// fmt.Errorf/fmt.Sprintf fills them, so the verbs survive; the tests pin that each
// translation keeps the source template's verbs in the same order.
var zhMsgCatalog = map[string]map[string]string{
	"%q 不是有效的 %s 范围":                   {"en": "%q is not a valid %s scope", "ko": "%q 은(는) 유효한 %s 범위가 아닙니다", "es": "%q no es un rango %s válido", "ja": "%q は有効な %s 範囲ではありません"},
	"ICP 不能为空":                         {"en": "ICP cannot be empty", "ko": "ICP 는 비워 둘 수 없습니다", "es": "ICP no puede estar vacío", "ja": "ICP は空にできません"},
	"企业关键词不能为空":                        {"en": "Company keyword cannot be empty", "ko": "기업 키워드는 비워 둘 수 없습니다", "es": "La palabra clave de la empresa no puede estar vacía", "ja": "企業キーワードは空にできません"},
	"不支持的范围类型: %s":                     {"en": "Unsupported scope type: %s", "ko": "지원하지 않는 범위 유형: %s", "es": "Tipo de rango no admitido: %s", "ja": "サポートされていない範囲タイプ: %s"},
	"空行":                               {"en": "Empty line", "ko": "빈 줄", "es": "Línea vacía", "ja": "空行です"},
	"无效 CIDR: %s":                      {"en": "Invalid CIDR: %s", "ko": "유효하지 않은 CIDR: %s", "es": "CIDR no válido: %s", "ja": "無効な CIDR: %s"},
	"无效 IP: %s":                        {"en": "Invalid IP: %s", "ko": "유효하지 않은 IP: %s", "es": "IP no válida: %s", "ja": "無効な IP: %s"},
	"主机名为空":                            {"en": "Hostname is empty", "ko": "호스트 이름이 비어 있습니다", "es": "El nombre de host está vacío", "ja": "ホスト名が空です"},
	"缺少主机名":                            {"en": "Missing hostname", "ko": "호스트 이름이 없습니다", "es": "Falta el nombre de host", "ja": "ホスト名がありません"},
	"域名超过 253 个字符":                     {"en": "Domain exceeds 253 characters", "ko": "도메인이 253자를 초과합니다", "es": "El dominio supera los 253 caracteres", "ja": "ドメインが 253 文字を超えています"},
	"域名至少需要两个标签":                       {"en": "Domain must have at least two labels", "ko": "도메인에는 레이블이 최소 두 개 필요합니다", "es": "El dominio debe tener al menos dos etiquetas", "ja": "ドメインには少なくとも 2 つのラベルが必要です"},
	"域名标签无效":                           {"en": "Invalid domain label", "ko": "유효하지 않은 도메인 레이블", "es": "Etiqueta de dominio no válida", "ja": "無効なドメインラベル"},
	"域名包含无效字符":                         {"en": "Domain contains invalid characters", "ko": "도메인에 유효하지 않은 문자가 포함되어 있습니다", "es": "El dominio contiene caracteres no válidos", "ja": "ドメインに無効な文字が含まれています"},
	"网段过宽(IPv4 需 >= /16): %s":          {"en": "Network range too broad (IPv4 requires >= /16): %s", "ko": "네트워크 대역이 너무 넓습니다(IPv4 는 >= /16 필요): %s", "es": "Rango de red demasiado amplio (IPv4 requiere >= /16): %s", "ja": "ネットワーク範囲が広すぎます (IPv4 は >= /16 が必要): %s"},
	"网段过宽(IPv6 需 >= /32): %s":          {"en": "Network range too broad (IPv6 requires >= /32): %s", "ko": "네트워크 대역이 너무 넓습니다(IPv6 는 >= /32 필요): %s", "es": "Rango de red demasiado amplio (IPv6 requiere >= /32): %s", "ja": "ネットワーク範囲が広すぎます (IPv6 は >= /32 が必要): %s"},
	"无法识别为有效域名/IP/CIDR: %s":            {"en": "Cannot recognize as a valid domain/IP/CIDR: %s", "ko": "유효한 도메인/IP/CIDR 로 인식할 수 없습니다: %s", "es": "No se reconoce como dominio/IP/CIDR válido: %s", "ja": "有効なドメイン/IP/CIDR として認識できません: %s"},
	"IP 段请用 CIDR 表示(如 1.2.3.0/24): %s": {"en": "Express IP ranges as CIDR (e.g. 1.2.3.0/24): %s", "ko": "IP 대역은 CIDR 로 표현하세요(예: 1.2.3.0/24): %s", "es": "Exprese los rangos de IP como CIDR (p. ej., 1.2.3.0/24): %s", "ja": "IP 範囲は CIDR で指定してください(例: 1.2.3.0/24): %s"},
	"不能用裸 TLD 作为范围: %s":                {"en": "A bare TLD cannot be used as a scope: %s", "ko": "최상위 도메인(TLD)만으로는 범위를 지정할 수 없습니다: %s", "es": "No se puede usar un TLD desnudo como rango: %s", "ja": "裸の TLD を範囲として使えません: %s"},

	"企业范围第 %d 条原始值过长: 最多 %d 个字符":                                 {"en": "Company scope entry %d has a raw value that is too long: at most %d characters", "ko": "기업 범위 %d번째 항목의 원본 값이 너무 깁니다: 최대 %d자", "es": "El valor original de la entrada %d del alcance de la empresa es demasiado largo: máximo %d caracteres", "ja": "企業範囲の %d 番目の項目の元の値が長すぎます: 最大 %d 文字"},
	"企业范围第 %d 条规范化值过长: 最多 %d 个字符":                                {"en": "Company scope entry %d has a normalized value that is too long: at most %d characters", "ko": "기업 범위 %d번째 항목의 정규화된 값이 너무 깁니다: 최대 %d자", "es": "El valor normalizado de la entrada %d del alcance de la empresa es demasiado largo: máximo %d caracteres", "ja": "企業範囲の %d 番目の項目の正規化された値が長すぎます: 最大 %d 文字"},
	"%d 条资产的 ip 字段不是合法 IP，已跳过 IP/CIDR 范围匹配（这些资产不会被网段规则归属到企业）：%s": {"en": "%d assets have an ip field that is not a valid IP; IP/CIDR scope matching was skipped (these assets will not be attributed to a company by network rules): %s", "ko": "%d개 자산의 ip 필드가 유효한 IP 가 아니어서 IP/CIDR 범위 매칭을 건너뛰었습니다(이 자산들은 네트워크 대역 규칙으로 기업에 귀속되지 않습니다): %s", "es": "%d activos tienen un campo ip que no es una IP válida; se omitió la coincidencia de rangos IP/CIDR (estos activos no se atribuirán a una empresa mediante reglas de red): %s", "ja": "%d 件の資産の ip フィールドが有効な IP ではないため、IP/CIDR 範囲の照合をスキップしました(これらの資産はネットワーク範囲ルールで企業に帰属されません): %s"},
	"、":       {"en": ", ", "ko": ", ", "es": ", ", "ja": ", "},
	" 等 %d 条": {"en": " (%d total)", "ko": " 등 %d개", "es": " (%d en total)", "ja": " など %d 件"},
	"企业范围包含 %d 条无效规则，未覆盖原有范围": {"en": "The company scope contains %d invalid rules; the original scope was not replaced", "ko": "기업 범위에 %d개의 유효하지 않은 규칙이 있어 기존 범위를 덮어쓰지 않았습니다", "es": "El alcance de la empresa contiene %d reglas no válidas; no se reemplazó el alcance original", "ja": "企業範囲に %d 件の無効なルールがあるため、元の範囲は上書きされていません"},
	"重新计算企业归属失败: %w":          {"en": "Failed to recompute company attribution: %w", "ko": "기업 귀속 재계산에 실패했습니다: %w", "es": "No se pudo recalcular la atribución de la empresa: %w", "ja": "企業の帰属の再計算に失敗しました: %w"},

	"通知渠道不存在":             {"en": "Notification channel not found", "ko": "알림 채널이 존재하지 않습니다", "es": "El canal de notificación no existe", "ja": "通知チャネルが存在しません"},
	"限流值不能为负":             {"en": "The rate limit cannot be negative", "ko": "속도 제한 값은 음수일 수 없습니다", "es": "El límite de velocidad no puede ser negativo", "ja": "レート制限値を負の値にできません"},
	"渠道已停用":               {"en": "Channel disabled", "ko": "채널이 비활성화되었습니다", "es": "Canal desactivado", "ja": "チャネルは無効化されています"},
	"序列化通知事件快照失败: %w":     {"en": "Failed to serialize the notification event snapshot: %w", "ko": "알림 이벤트 스냅샷 직렬화에 실패했습니다: %w", "es": "No se pudo serializar la instantánea del evento de notificación: %w", "ja": "通知イベントスナップショットのシリアライズに失敗しました: %w"},
	"投递 %d 不存在或当前状态不允许重发": {"en": "Delivery %d does not exist or its current state does not allow resending", "ko": "%d번 배송이 존재하지 않거나 현재 상태에서 재발송할 수 없습니다", "es": "La entrega %d no existe o su estado actual no permite reenviar", "ja": "配信 %d が存在しないか、現在の状態では再送できません"},

	"域名(全等)":  {"en": "Domain (exact)", "ko": "도메인(완전 일치)", "es": "Dominio (exacto)", "ja": "ドメイン(完全一致)"},
	"IP(全等)":  {"en": "IP (exact)", "ko": "IP(완전 일치)", "es": "IP (exacto)", "ja": "IP(完全一致)"},
	"URL(全等)": {"en": "URL (exact)", "ko": "URL(완전 일치)", "es": "URL (exacto)", "ja": "URL(完全一致)"},
	"域名(模糊)":  {"en": "Domain (fuzzy)", "ko": "도메인(부분 일치)", "es": "Dominio (difuso)", "ja": "ドメイン(部分一致)"},
	"IP(模糊)":  {"en": "IP (fuzzy)", "ko": "IP(부분 일치)", "es": "IP (difuso)", "ja": "IP(部分一致)"},
	"URL(模糊)": {"en": "URL (fuzzy)", "ko": "URL(부분 일치)", "es": "URL (difuso)", "ja": "URL(部分一致)"},
	"CIDR 网段": {"en": "CIDR range", "ko": "CIDR 대역", "es": "Rango CIDR", "ja": "CIDR 範囲"},

	"命中资产拦截规则 [%s: %s]": {"en": "Matched asset interception rule [%s: %s]", "ko": "자산 차단 규칙에 일치했습니다 [%s: %s]", "es": "Se coincidió con la regla de interceptación de activos [%s: %s]", "ja": "資産インターセプトルールに一致しました [%s: %s]"},
	"（%s）":              {"en": " (%s)", "ko": " (%s)", "es": " (%s)", "ja": " (%s)"},
	"不在任务允许(白名单)范围内，不允许测试": {"en": "Not within the task allowlist; testing is not permitted", "ko": "작업 허용(화이트리스트) 범위에 없어 테스트할 수 없습니다", "es": "No está dentro del rango permitido (lista blanca) de la tarea; no se permite probar", "ja": "タスクの許可(ホワイトリスト)範囲外のため、テストできません"},
	"资产#%d[%s] %s":   {"en": "Asset#%d[%s] %s", "ko": "자산#%d[%s] %s", "es": "Activo#%d[%s] %s", "ja": "資産#%d[%s] %s"},
	"资产 %s %s，已禁止插入": {"en": "Asset %s %s was not inserted (blocked)", "ko": "자산 %s %s 은(는) 삽입이 금지되었습니다", "es": "El activo %s %s no se insertó (bloqueado)", "ja": "資産 %s %s は挿入が禁止されています"},
	"(未知)":           {"en": "(unknown)", "ko": "(알 수 없음)", "es": "(desconocido)", "ja": "(不明)"},

	"证据正文校验失败: %s":                {"en": "Evidence body verification failed: %s", "ko": "증거 본문 검증에 실패했습니다: %s", "es": "Falló la verificación del cuerpo de la evidencia: %s", "ja": "証拠本文の検証に失敗しました: %s"},
	"正文不完整: 预期 %d 字节，读取 %d 字节":    {"en": "Body incomplete: expected %d bytes, read %d bytes", "ko": "본문이 불완전합니다: 예상 %d 바이트, 읽은 %d 바이트", "es": "Cuerpo incompleto: se esperaban %d bytes, se leyeron %d bytes", "ja": "本文が不完全です: 期待 %d バイト、読み取り %d バイト"},
	"原始流量正文哈希不匹配":                 {"en": "Raw traffic body hash mismatch", "ko": "원본 트래픽 본문 해시가 일치하지 않습니다", "es": "El hash del cuerpo del tráfico original no coincide", "ja": "元のトラフィック本文のハッシュが一致しません"},
	"side 必须为 request 或 response": {"en": "side must be request or response", "ko": "side 는 request 또는 response 여야 합니다", "es": "side debe ser request o response", "ja": "side は request または response にしてください"},
	"归档证据快照元数据哈希不匹配":              {"en": "Archived evidence snapshot metadata hash mismatch", "ko": "아카이브 증거 스냅샷 메타데이터 해시가 일치하지 않습니다", "es": "El hash de los metadatos de la instantánea de evidencia archivada no coincide", "ja": "アーカイブされた証拠スナップショットのメタデータハッシュが一致しません"},

	"流量录制存储不可用": {"en": "Traffic capture storage is unavailable", "ko": "트래픽 녹화 저장소를 사용할 수 없습니다", "es": "El almacenamiento de captura de tráfico no está disponible", "ja": "トラフィック記録ストレージを利用できません"},
	"流量 %s: %w": {"en": "Traffic %s: %w", "ko": "트래픽 %s: %w", "es": "Tráfico %s: %w", "ja": "トラフィック %s: %w"},
	"旧流量路径无效":   {"en": "Invalid legacy traffic path", "ko": "이전 트래픽 경로가 유효하지 않습니다", "es": "Ruta de tráfico heredada no válida", "ja": "旧トラフィックパスが無効です"},
	"旧流量报文头过大":  {"en": "Legacy traffic header is too large", "ko": "이전 트래픽 헤더가 너무 큽니다", "es": "La cabecera del tráfico heredado es demasiado grande", "ja": "旧トラフィックのヘッダーが大きすぎます"},
}

// TrMsg translates a user-facing Chinese source message into the active output
// language. It returns the original unchanged when the active language is "zh",
// when the key is absent, or when the entry has no translation for that language.
// Callers must pass the format template before fmt.Errorf/fmt.Sprintf fills it.
func TrMsg(zh string) string {
	lang := ActiveLanguage()
	if lang == "zh" {
		return zh
	}
	if m, ok := zhMsgCatalog[zh]; ok {
		if v, ok := m[lang]; ok && v != "" {
			return v
		}
	}
	return zh
}
