import { isLocale, LOCALE_COOKIE, LOCALE_STORAGE_KEY, type Locale, resolveLocale } from "@/i18n/config";
import { getClientCookie } from "@/lib/cookie.client";
import { getLocalStorageValue } from "@/lib/local-storage.client";

// 클라이언트 측 오류 문구 카탈로그. lib/api.ts 는 React 컴포넌트 밖에서 throw 하므로
// next-intl 의 useTranslations 훅을 쓸 수 없다. 대신 표시 언어를 UI 와 같은 소스
// (localStorage → 쿠키 → 빌드 기본값)에서 읽어, api.ts 의 오류가 선택한 언어로 나가게
// 한다. 서버가 번역해 돌려주는 오류(ApiError.message)는 그대로 두고, 브라우저에서만
// 만들어지는 문구(업로드/다운로드 실패, 401)만 여기서 번역한다.
//
// 왜 web/messages/*.json 에 넣지 않는가: 그 파일들은 UI 컴포넌트 네임스페이스로 구성돼
// 있고 check:i18n 가드가 ko 기준 5개 로케일의 키 집합 일치를 강제한다. 여기에 새
// 네임스페이스를 넣으면 5개 로케일을 모두 고쳐야 하는데 api.ts 문구는 소수라 별도 표가
// 더 작고 안전하다. 문구를 추가할 때는 5개 언어를 함께 채워야 한다(테스트가 강제).

// apiClientMsgCatalog 는 한국어 원문 → 언어별 번역. "ko" 는 키 자체라 저장하지 않는다.
const apiClientMsgCatalog: Record<string, Record<Exclude<Locale, "ko">, string>> = {
  "인증되지 않았습니다": {
    en: "Not authenticated",
    zh: "未通过身份验证",
    es: "No autenticado",
    ja: "認証されていません",
  },
  "업로드 실패: ": {
    en: "Upload failed: ",
    zh: "上传失败：",
    es: "Error al subir: ",
    ja: "アップロードに失敗しました: ",
  },
  "업로드 실패(": {
    en: "Upload failed (",
    zh: "上传失败（",
    es: "Error al subir (",
    ja: "アップロードに失敗しました (",
  },
  "다운로드 실패: ": {
    en: "Download failed: ",
    zh: "下载失败：",
    es: "Error al descargar: ",
    ja: "ダウンロードに失敗しました: ",
  },
  "다운로드 실패": {
    en: "Download failed",
    zh: "下载失败",
    es: "Error al descargar",
    ja: "ダウンロードに失敗しました",
  },
  "범위 값을 입력해 주세요": {
    en: "Enter a scope value",
    zh: "请输入范围值",
    es: "Introduzca un valor de alcance",
    ja: "範囲の値を入力してください",
  },
  "최대 ": {
    en: "You can enter up to ",
    zh: "最多可输入 ",
    es: "Puede introducir hasta ",
    ja: "最大 ",
  },
  "자까지 입력할 수 있습니다": {
    en: " characters",
    zh: " 个字符",
    es: " caracteres",
    ja: " 文字まで入力できます",
  },
  "공백 없이 유효한 도메인 또는 URL을 입력해 주세요": {
    en: "Enter a valid domain or URL without spaces",
    zh: "请输入不含空格的有效域名或 URL",
    es: "Introduzca un dominio o URL válido sin espacios",
    ja: "空白を含まない有効なドメインまたは URL を入力してください",
  },
  "유효한 도메인 또는 URL을 입력해 주세요": {
    en: "Enter a valid domain or URL",
    zh: "请输入有效的域名或 URL",
    es: "Introduzca un dominio o URL válido",
    ja: "有効なドメインまたは URL を入力してください",
  },
  "유효한 IP를 입력해 주세요": {
    en: "Enter a valid IP",
    zh: "请输入有效的 IP",
    es: "Introduzca una IP válida",
    ja: "有効な IP を入力してください",
  },
  "유효한 CIDR 대역을 입력해 주세요": {
    en: "Enter a valid CIDR range",
    zh: "请输入有效的 CIDR 网段",
    es: "Introduzca un rango CIDR válido",
    ja: "有効な CIDR 範囲を入力してください",
  },
  "IPv4 대역 프리픽스는 /16 ~ /32 범위여야 합니다": {
    en: "IPv4 range prefix must be between /16 and /32",
    zh: "IPv4 网段前缀必须在 /16 到 /32 之间",
    es: "El prefijo del rango IPv4 debe estar entre /16 y /32",
    ja: "IPv4 範囲のプレフィックスは /16 から /32 の範囲である必要があります",
  },
  "IPv6 대역 프리픽스는 /32 ~ /128 범위여야 합니다": {
    en: "IPv6 range prefix must be between /32 and /128",
    zh: "IPv6 网段前缀必须在 /32 到 /128 之间",
    es: "El prefijo del rango IPv6 debe estar entre /32 y /128",
    ja: "IPv6 範囲のプレフィックスは /32 から /128 の範囲である必要があります",
  },
  " 의 다운로드 내용 예시입니다.": {
    en: " — sample download content.",
    zh: " 的下载内容示例。",
    es: " — contenido de descarga de ejemplo.",
    ja: " のダウンロード内容の例です。",
  },
};

// activeLocale 은 UI 와 동일한 우선순위로 표시 언어를 읽는다(서버 렌더에서는 빌드 기본값).
function activeLocale(): Locale {
  if (typeof window === "undefined") return resolveLocale();
  const stored = getLocalStorageValue(LOCALE_STORAGE_KEY);
  if (isLocale(stored)) return stored;
  const cookie = getClientCookie(LOCALE_COOKIE);
  if (isLocale(cookie)) return cookie;
  return resolveLocale();
}

// clientMsg 는 한국어 원문 조각을 활성 표시 언어로 바꾼다. 표에 없거나 한국어이면 원문
// 그대로 돌려주므로, 새 문구가 생겨도 최소한 한국어로 읽히고 깨지지 않는다.
export function clientMsg(ko: string): string {
  const locale = activeLocale();
  if (locale === "ko") return ko;
  return apiClientMsgCatalog[ko]?.[locale] ?? ko;
}

// clientMsgWithArg 는 접두 조각·런타임 값·접미 조각을 이어 붙인다. 값(상태 코드·경로)은
// 번역하지 않는다. 조각이 표에 없으면 원문 한국어를 그대로 쓴다.
export function clientMsgWithArg(koPrefix: string, arg: string | number, koSuffix = ""): string {
  const locale = activeLocale();
  if (locale === "ko") return koPrefix + arg + koSuffix;
  const prefix = apiClientMsgCatalog[koPrefix]?.[locale] ?? koPrefix;
  const suffix = koSuffix ? (apiClientMsgCatalog[koSuffix]?.[locale] ?? koSuffix) : "";
  return prefix + arg + suffix;
}
