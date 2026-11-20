package selfupdate

// 이 포크는 사용자 대면 출력을 선택한 언어로 강제한다. selfupdate 패키지의 오류·진행
// 문구는 그대로 UI 에 노출되므로(update-card 의 error/progress, boot_notice) 번역 표를
// 거쳐야 한다. server 패키지의 errCatalog 를 재사용할 수 없는 이유는 순환 임포트
// (server → selfupdate) 때문이고, 그래서 intercept/label_i18n.go 와 같은 방식으로
// 이 패키지 안에 자체 표를 둔다. 키는 소스의 한국어 원문 그대로다.

import (
	"errors"
	"fmt"

	"github.com/Autumn-27/artex/config"
)

// updateMsgCatalog 는 한국어 원문 → 언어별 번역. "ko" 는 키 자체라 저장하지 않는다.
var updateMsgCatalog = map[string]map[string]string{
	// ---- GitHub 조회/리다이렉트 (github.go) ----
	"리다이렉트 횟수가 너무 많습니다": {
		"en": "Too many redirects",
		"zh": "重定向次数过多",
		"es": "Demasiadas redirecciones",
		"ja": "リダイレクト回数が多すぎます",
	},
	"HTTPS 가 아닌 주소를 거부합니다: %s": {
		"en": "Refusing a non-HTTPS URL: %s",
		"zh": "拒绝非 HTTPS 地址：%s",
		"es": "Se rechaza una URL que no es HTTPS: %s",
		"ja": "HTTPS でない URL を拒否します: %s",
	},
	"GitHub 도메인이 아닌 주소를 거부합니다: %s": {
		"en": "Refusing a URL outside the GitHub domains: %s",
		"zh": "拒绝不属于 GitHub 域名的地址：%s",
		"es": "Se rechaza una URL fuera de los dominios de GitHub: %s",
		"ja": "GitHub のドメイン外の URL を拒否します: %s",
	},
	"GitHub 에 접속하지 못했습니다(시스템 설정에서 전역 프록시를 설정할 수 있습니다): %w": {
		"en": "Could not reach GitHub (you can set a global proxy in System Settings): %w",
		"zh": "无法连接 GitHub（可在系统设置中配置全局代理）：%w",
		"es": "No se pudo conectar con GitHub (puede configurar un proxy global en Ajustes del sistema): %w",
		"ja": "GitHub に接続できませんでした（システム設定でグローバルプロキシを設定できます）: %w",
	},
	"GitHub API 요청 제한에 걸렸습니다(시간당 60회). 잠시 후 다시 시도하세요": {
		"en": "Hit the GitHub API rate limit (60 requests per hour). Try again shortly",
		"zh": "已触发 GitHub API 请求限制（每小时 60 次）。请稍后重试",
		"es": "Se alcanzó el límite de solicitudes de la API de GitHub (60 por hora). Vuelva a intentarlo en breve",
		"ja": "GitHub API のレート制限（1 時間あたり 60 回）に達しました。しばらくしてから再試行してください",
	},
	"저장소 %s 에 아직 정식 릴리스가 없습니다": {
		"en": "Repository %s has no published release yet",
		"zh": "仓库 %s 尚未发布正式版本",
		"es": "El repositorio %s aún no tiene una versión publicada",
		"ja": "リポジトリ %s にはまだ正式リリースがありません",
	},
	"GitHub 응답 코드 %d": {
		"en": "GitHub response code %d",
		"zh": "GitHub 响应代码 %d",
		"es": "Código de respuesta de GitHub %d",
		"ja": "GitHub の応答コード %d",
	},
	"Release 해석 실패: %w": {
		"en": "Failed to parse the release: %w",
		"zh": "解析 Release 失败：%w",
		"es": "No se pudo analizar la versión: %w",
		"ja": "リリースの解析に失敗しました: %w",
	},
	"Release 에 tag 가 없습니다": {
		"en": "The release has no tag",
		"zh": "Release 中没有 tag",
		"es": "La versión no tiene etiqueta",
		"ja": "リリースに tag がありません",
	},

	// ---- 다운로드/검증/압축 해제 (stage.go) ----
	"이 버전은 %s/%s 용 릴리스 패키지를 제공하지 않습니다(%s 누락)": {
		"en": "This version has no release package for %s/%s (%s is missing)",
		"zh": "此版本没有适用于 %s/%s 的发布包（缺少 %s）",
		"es": "Esta versión no tiene paquete de publicación para %s/%s (falta %s)",
		"ja": "このバージョンには %s/%s 用のリリースパッケージがありません（%s が欠落）",
	},
	"%s 에 %s 항목이 없어 검증되지 않은 바이너리 설치를 거부합니다": {
		"en": "%s has no entry for %s, so installing an unverified binary is refused",
		"zh": "%s 中没有 %s 条目，因此拒绝安装未经验证的二进制文件",
		"es": "%s no tiene una entrada para %s, por lo que se rechaza instalar un binario sin verificar",
		"ja": "%s に %s の項目がないため、検証されていないバイナリのインストールを拒否します",
	},
	"%s 내려받는 중(%s)…": {
		"en": "Downloading %s (%s)…",
		"zh": "正在下载 %s（%s）…",
		"es": "Descargando %s (%s)…",
		"ja": "%s をダウンロード中（%s）…",
	},
	"SHA256 이 일치하지 않습니다: 기대값 %s, 실제값 %s(다운로드가 손상됐거나 변조됨)": {
		"en": "SHA256 mismatch: expected %s, got %s (the download was corrupted or tampered with)",
		"zh": "SHA256 不匹配：期望 %s，实际 %s（下载已损坏或被篡改）",
		"es": "SHA256 no coincide: se esperaba %s, se obtuvo %s (la descarga se corrompió o se alteró)",
		"ja": "SHA256 が一致しません: 期待値 %s、実際の値 %s（ダウンロードが破損または改ざんされています）",
	},
	"새 버전이 현재 시스템에서 실행되지 않습니다: %w": {
		"en": "The new version does not run on this system: %w",
		"zh": "新版本无法在当前系统上运行：%w",
		"es": "La nueva versión no se ejecuta en este sistema: %w",
		"ja": "新しいバージョンがこのシステムで実行できません: %w",
	},
	"새 바이너리 체크섬 계산 실패: %w": {
		"en": "Failed to compute the new binary's checksum: %w",
		"zh": "计算新二进制文件的校验和失败：%w",
		"es": "No se pudo calcular la suma de verificación del nuevo binario: %w",
		"ja": "新しいバイナリのチェックサム計算に失敗しました: %w",
	},
	"체크섬 기록 실패: %w": {
		"en": "Failed to record the checksum: %w",
		"zh": "记录校验和失败：%w",
		"es": "No se pudo registrar la suma de verificación: %w",
		"ja": "チェックサムの記録に失敗しました: %w",
	},
	"새 버전 준비 실패: %w": {
		"en": "Failed to prepare the new version: %w",
		"zh": "准备新版本失败：%w",
		"es": "No se pudo preparar la nueva versión: %w",
		"ja": "新しいバージョンの準備に失敗しました: %w",
	},
	"이 릴리스에 %s 항목이 없어 무결성을 검증할 수 없으므로 업데이트를 거부합니다": {
		"en": "This release has no %s entry, so integrity cannot be verified and the update is refused",
		"zh": "此发布缺少 %s 条目，无法验证完整性，因此拒绝更新",
		"es": "Esta versión no tiene una entrada %s, por lo que no se puede verificar la integridad y se rechaza la actualización",
		"ja": "このリリースには %s の項目がないため整合性を検証できず、更新を拒否します",
	},
	"%s 내려받기 실패: %w": {
		"en": "Failed to download %s: %w",
		"zh": "下载 %s 失败：%w",
		"es": "No se pudo descargar %s: %w",
		"ja": "%s のダウンロードに失敗しました: %w",
	},
	"%s 읽기 실패: %w": {
		"en": "Failed to read %s: %w",
		"zh": "读取 %s 失败：%w",
		"es": "No se pudo leer %s: %w",
		"ja": "%s の読み取りに失敗しました: %w",
	},
	"%s 내용이 비어 있거나 형식을 인식할 수 없습니다": {
		"en": "%s is empty or its format is not recognized",
		"zh": "%s 内容为空或格式无法识别",
		"es": "%s está vacío o su formato no se reconoce",
		"ja": "%s の内容が空か、形式を認識できません",
	},
	"임시 파일 생성 실패: %w": {
		"en": "Failed to create a temporary file: %w",
		"zh": "创建临时文件失败：%w",
		"es": "No se pudo crear un archivo temporal: %w",
		"ja": "一時ファイルの作成に失敗しました: %w",
	},
	"다운로드가 중단되었습니다: %w": {
		"en": "The download was interrupted: %w",
		"zh": "下载已中断：%w",
		"es": "La descarga se interrumpió: %w",
		"ja": "ダウンロードが中断されました: %w",
	},
	"디스크 기록 실패: %w": {
		"en": "Failed to write to disk: %w",
		"zh": "写入磁盘失败：%w",
		"es": "No se pudo escribir en el disco: %w",
		"ja": "ディスクへの書き込みに失敗しました: %w",
	},
	"다운로드가 완전하지 않습니다: 기대 %d바이트, 실제 %d바이트": {
		"en": "The download is incomplete: expected %d bytes, got %d bytes",
		"zh": "下载不完整：期望 %d 字节，实际 %d 字节",
		"es": "La descarga está incompleta: se esperaban %d bytes, se obtuvieron %d bytes",
		"ja": "ダウンロードが不完全です: 期待 %d バイト、実際 %d バイト",
	},
	"릴리스 패키지 열기 실패: %w": {
		"en": "Failed to open the release package: %w",
		"zh": "打开发布包失败：%w",
		"es": "No se pudo abrir el paquete de publicación: %w",
		"ja": "リリースパッケージを開けませんでした: %w",
	},
	"새 바이너리 쓰기 실패: %w": {
		"en": "Failed to write the new binary: %w",
		"zh": "写入新二进制文件失败：%w",
		"es": "No se pudo escribir el nuevo binario: %w",
		"ja": "新しいバイナリの書き込みに失敗しました: %w",
	},
	"%s 압축 해제 실패: %w": {
		"en": "Failed to extract %s: %w",
		"zh": "解压 %s 失败：%w",
		"es": "No se pudo extraer %s: %w",
		"ja": "%s の展開に失敗しました: %w",
	},
	"릴리스 패키지 안의 실행 파일이 %s 보다 커서 압축 해제를 거부합니다": {
		"en": "The executable inside the release package is larger than %s, so extraction is refused",
		"zh": "发布包内的可执行文件大于 %s，因此拒绝解压",
		"es": "El ejecutable del paquete de publicación supera %s, por lo que se rechaza la extracción",
		"ja": "リリースパッケージ内の実行ファイルが %s を超えているため、展開を拒否します",
	},
	"릴리스 패키지 안의 %s 파일이 비어 있습니다": {
		"en": "The %s file inside the release package is empty",
		"zh": "发布包内的 %s 文件为空",
		"es": "El archivo %s dentro del paquete de publicación está vacío",
		"ja": "リリースパッケージ内の %s ファイルが空です",
	},
	"릴리스 패키지에서 %s 파일을 찾지 못했습니다": {
		"en": "Could not find the %s file in the release package",
		"zh": "在发布包中找不到 %s 文件",
		"es": "No se encontró el archivo %s en el paquete de publicación",
		"ja": "リリースパッケージ内に %s ファイルが見つかりませんでした",
	},
	"프로그램 디렉터리 %s 에 쓸 수 없어 자동 업데이트를 할 수 없습니다(권한을 확인하거나 수동 업데이트를 사용하세요): %w": {
		"en": "The program directory %s is not writable, so automatic updates are unavailable (check permissions or update manually): %w",
		"zh": "程序目录 %s 不可写，因此无法自动更新（请检查权限或手动更新）：%w",
		"es": "El directorio del programa %s no admite escritura, por lo que la actualización automática no está disponible (compruebe los permisos o actualice manualmente): %w",
		"ja": "プログラムディレクトリ %s に書き込めないため自動更新を利用できません（権限を確認するか手動で更新してください）: %w",
	},
	"내려받는 중 %s / %s": {
		"en": "Downloading %s / %s",
		"zh": "正在下载 %s / %s",
		"es": "Descargando %s / %s",
		"ja": "%s / %s をダウンロード中",
	},
	"체크섬 목록을 가져오는 중…": {
		"en": "Fetching the checksum list…",
		"zh": "正在获取校验和列表…",
		"es": "Obteniendo la lista de sumas de verificación…",
		"ja": "チェックサム一覧を取得中…",
	},
	"SHA256 검증 중…": {
		"en": "Verifying SHA256…",
		"zh": "正在验证 SHA256…",
		"es": "Verificando SHA256…",
		"ja": "SHA256 を検証中…",
	},
	"압축을 풀고 스모크 테스트 중…": {
		"en": "Extracting and running the smoke test…",
		"zh": "正在解压并执行冒烟测试…",
		"es": "Extrayendo y ejecutando la prueba de humo…",
		"ja": "展開してスモークテストを実行中…",
	},
	"경고: 업데이트 표시 파일 기록에 실패해 이번 업데이트는 자동 롤백 보호를 받지 못합니다": {
		"en": "Warning: failed to write the upgrade marker, so this update has no automatic rollback protection",
		"zh": "警告：写入更新标记失败，本次更新不受自动回滚保护",
		"es": "Advertencia: no se pudo escribir el marcador de actualización, por lo que esta actualización no tiene protección de reversión automática",
		"ja": "警告: 更新マーカーの書き込みに失敗したため、この更新は自動ロールバックの保護を受けられません",
	},
	"새 버전이 준비되었습니다. 재시작 중…": {
		"en": "The new version is ready. Restarting…",
		"zh": "新版本已就绪，正在重启…",
		"es": "La nueva versión está lista. Reiniciando…",
		"ja": "新しいバージョンの準備ができました。再起動中…",
	},

	// ---- 부트스트랩/롤백 (bootstrap.go) ----
	"새 버전 시작에 실패해 이전 버전으로 롤백했습니다(%s)": {
		"en": "The new version failed to start, so it was rolled back to the previous version (%s)",
		"zh": "新版本启动失败，已回滚到上一版本（%s）",
		"es": "La nueva versión no pudo iniciarse, por lo que se revirtió a la versión anterior (%s)",
		"ja": "新しいバージョンの起動に失敗したため、以前のバージョン（%s）にロールバックしました",
	},
	"체크섬 읽기 실패: %w": {
		"en": "Failed to read the checksum: %w",
		"zh": "读取校验和失败：%w",
		"es": "No se pudo leer la suma de verificación: %w",
		"ja": "チェックサムの読み取りに失敗しました: %w",
	},
	"체크섬 계산 실패: %w": {
		"en": "Failed to compute the checksum: %w",
		"zh": "计算校验和失败：%w",
		"es": "No se pudo calcular la suma de verificación: %w",
		"ja": "チェックサムの計算に失敗しました: %w",
	},
	"SHA256 이 일치하지 않습니다(다운로드가 손상됐거나 변조됨)": {
		"en": "SHA256 mismatch (the download was corrupted or tampered with)",
		"zh": "SHA256 不匹配（下载已损坏或被篡改）",
		"es": "SHA256 no coincide (la descarga se corrompió o se alteró)",
		"ja": "SHA256 が一致しません（ダウンロードが破損または改ざんされています）",
	},
	"실행 권한 부여 실패: %w": {
		"en": "Failed to grant execute permission: %w",
		"zh": "授予执行权限失败：%w",
		"es": "No se pudo conceder permiso de ejecución: %w",
		"ja": "実行権限の付与に失敗しました: %w",
	},
	"스모크 테스트 시간 초과(새 바이너리가 응답하지 않음)": {
		"en": "The smoke test timed out (the new binary is not responding)",
		"zh": "冒烟测试超时（新二进制文件无响应）",
		"es": "La prueba de humo agotó el tiempo de espera (el nuevo binario no responde)",
		"ja": "スモークテストがタイムアウトしました（新しいバイナリが応答しません）",
	},
	"스모크 테스트 실패: %v: %s": {
		"en": "Smoke test failed: %v: %s",
		"zh": "冒烟测试失败：%v：%s",
		"es": "La prueba de humo falló: %v: %s",
		"ja": "スモークテストに失敗しました: %v: %s",
	},
	"이전 백업 %s 정리 실패: %w": {
		"en": "Failed to clean up the previous backup %s: %w",
		"zh": "清理旧的备份 %s 失败：%w",
		"es": "No se pudo limpiar la copia de seguridad anterior %s: %w",
		"ja": "以前のバックアップ %s の整理に失敗しました: %w",
	},
	"현재 버전 백업 실패: %w": {
		"en": "Failed to back up the current version: %w",
		"zh": "备份当前版本失败：%w",
		"es": "No se pudo respaldar la versión actual: %w",
		"ja": "現在のバージョンのバックアップに失敗しました: %w",
	},
	"새 버전 설치 실패(%v), 현재 버전 복구도 실패: %w": {
		"en": "Failed to install the new version (%v), and restoring the current version also failed: %w",
		"zh": "安装新版本失败（%v），恢复当前版本也失败：%w",
		"es": "No se pudo instalar la nueva versión (%v) y también falló la restauración de la versión actual: %w",
		"ja": "新しいバージョンのインストールに失敗し（%v）、現在のバージョンの復元にも失敗しました: %w",
	},
	"새 버전 설치 실패: %w": {
		"en": "Failed to install the new version: %w",
		"zh": "安装新版本失败：%w",
		"es": "No se pudo instalar la nueva versión: %w",
		"ja": "新しいバージョンのインストールに失敗しました: %w",
	},
	"롤백할 백업이 없습니다(%s): %w": {
		"en": "There is no backup to roll back to (%s): %w",
		"zh": "没有可回滚的备份（%s）：%w",
		"es": "No hay copia de seguridad a la que revertir (%s): %w",
		"ja": "ロールバックできるバックアップがありません（%s）: %w",
	},
	"실패한 버전 이동 실패: %w": {
		"en": "Failed to move the failed version: %w",
		"zh": "移动失败的版本失败：%w",
		"es": "No se pudo mover la versión fallida: %w",
		"ja": "失敗したバージョンの移動に失敗しました: %w",
	},
	"이전 버전 복구 실패: %w": {
		"en": "Failed to restore the previous version: %w",
		"zh": "恢复上一版本失败：%w",
		"es": "No se pudo restaurar la versión anterior: %w",
		"ja": "以前のバージョンの復元に失敗しました: %w",
	},
	"이전 버전을 실행할 수 없어 롤백을 거부합니다: %w": {
		"en": "The previous version cannot be executed, so the rollback is refused: %w",
		"zh": "上一版本无法执行，因此拒绝回滚：%w",
		"es": "La versión anterior no se puede ejecutar, por lo que se rechaza la reversión: %w",
		"ja": "以前のバージョンを実行できないため、ロールバックを拒否します: %w",
	},
	"현재 버전 이동 실패: %w": {
		"en": "Failed to move the current version: %w",
		"zh": "移动当前版本失败：%w",
		"es": "No se pudo mover la versión actual: %w",
		"ja": "現在のバージョンの移動に失敗しました: %w",
	},
	"이전 버전 설치 실패: %w": {
		"en": "Failed to install the previous version: %w",
		"zh": "安装上一版本失败：%w",
		"es": "No se pudo instalar la versión anterior: %w",
		"ja": "以前のバージョンのインストールに失敗しました: %w",
	},
	"새 버전 검증에 실패해 폐기했습니다: ": {
		"en": "The new version failed verification and was discarded: ",
		"zh": "新版本验证失败，已丢弃：",
		"es": "La nueva versión no superó la verificación y se descartó: ",
		"ja": "新しいバージョンの検証に失敗したため破棄しました: ",
	},
	"버전 교체에 실패했습니다: ": {
		"en": "Failed to swap the version: ",
		"zh": "版本替换失败：",
		"es": "No se pudo intercambiar la versión: ",
		"ja": "バージョンの入れ替えに失敗しました: ",
	},
	"새 버전 시작에 실패하고 롤백도 실패했습니다: ": {
		"en": "The new version failed to start and the rollback also failed: ",
		"zh": "新版本启动失败，回滚也失败：",
		"es": "La nueva versión no pudo iniciarse y la reversión también falló: ",
		"ja": "新しいバージョンの起動に失敗し、ロールバックにも失敗しました: ",
	},
	"롤백할 이전 버전이 없습니다(": {
		"en": "There is no previous version to roll back to (",
		"zh": "没有可回滚的上一版本（",
		"es": "No hay versión anterior a la que revertir (",
		"ja": "ロールバックできる以前のバージョンがありません（",
	},

	// ---- 경로/기타 (selfupdate.go) ----
	"실행 파일 위치 확인 실패: %w": {
		"en": "Failed to locate the executable: %w",
		"zh": "无法确定可执行文件的位置：%w",
		"es": "No se pudo localizar el ejecutable: %w",
		"ja": "実行ファイルの場所を特定できませんでした: %w",
	},
	"알 수 없는 버전": {
		"en": "unknown version",
		"zh": "未知版本",
		"es": "versión desconocida",
		"ja": "不明なバージョン",
	},
}

// trMsg 는 한국어 원문을 활성 출력 언어로 치환한다. 표에 없거나 한국어이면 원문 그대로.
func trMsg(ko string) string {
	if m, ok := updateMsgCatalog[ko]; ok {
		if v, ok2 := m[config.ActiveLanguage()]; ok2 && v != "" {
			return v
		}
	}
	return ko
}

// errf 는 fmt.Errorf 와 같지만 포맷 문자열을 먼저 번역한다. %w 로 감싼 하위 오류는
// 그대로 전파된다.
func errf(format string, args ...any) error {
	return fmt.Errorf(trMsg(format), args...)
}

// errs 는 errors.New 와 같지만 메시지를 먼저 번역한다.
func errs(msg string) error {
	return errors.New(trMsg(msg))
}

// msgf 는 fmt.Sprintf 와 같지만 포맷 문자열을 먼저 번역한다. 진행 메시지처럼
// UI 에 그대로 노출되는 문자열에 쓴다.
func msgf(format string, args ...any) string {
	return fmt.Sprintf(trMsg(format), args...)
}
