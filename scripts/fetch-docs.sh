#!/usr/bin/env bash
# fetch-docs.sh — 토스증권 Open API 공식 문서 원본을 docs/api/ 로 가져온다.
#
#   ./scripts/fetch-docs.sh
#
# 동작:
#   1. llms.txt(https://developers.tossinvest.com/llms.txt) 가 안내하는 원본 4개를 임시
#      디렉토리에 다운로드 — 하나라도 실패하면 아무것도 반영하지 않고 중단
#   2. JSON 2개는 jq 로 pretty-print (git diff 가능하도록), md 2개는 그대로
#   3. 내용이 바뀐 파일만 docs/api/ 로 교체
#   4. 교체된 파일에 대해 docs/api/README.md 의 "가져온 버전" 표 행(버전·날짜) 갱신
#
# 의존: curl, jq
# TOSS_DOCS_BASE 환경변수로 원본 base URL 을 바꿀 수 있다(테스트용).
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUT="$ROOT/docs/api"
README="$OUT/README.md"
BASE="${TOSS_DOCS_BASE:-https://openapi.tossinvest.com/openapi-docs}"

for cmd in curl jq; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "error: '$cmd' 가 필요합니다" >&2
    exit 1
  fi
done

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

# fetch <url> <tmpfile>
fetch() {
  echo "GET $1"
  curl -fsSL "$1" -o "$2"
}

# 1) 전부 다운로드 (하나라도 실패하면 set -e 로 여기서 중단 → docs/api 미변경)
fetch "$BASE/latest/openapi.json"            "$TMP/openapi.raw.json"
fetch "$BASE/latest/asyncapi.json"           "$TMP/asyncapi.raw.json"
fetch "$BASE/overview.md"                    "$TMP/overview.md"
fetch "$BASE/latest/api-reference/README.md" "$TMP/api-reference.md"

# 2) pretty-print (키 순서 유지)
jq . "$TMP/openapi.raw.json"  > "$TMP/openapi.json"
jq . "$TMP/asyncapi.raw.json" > "$TMP/asyncapi.json"

OPENAPI_VER="$(jq -r .info.version "$TMP/openapi.json")"
ASYNCAPI_VER="$(jq -r .info.version "$TMP/asyncapi.json")"
TODAY="$(date +%Y-%m-%d)"

mkdir -p "$OUT"

# update_readme_row <name> <version> — README 버전 표에서 해당 파일 행의 버전·날짜를 바꾼다
update_readme_row() {
  local name="$1" ver="$2"
  [[ -f "$README" ]] || return 0
  sed -E "s#^\| \`$name\` \| [^|]* \| [^|]* \|#| \`$name\` | $ver | $TODAY |#" "$README" > "$TMP/README.md"
  mv "$TMP/README.md" "$README"
}

# install_doc <tmpfile> <name> <version> — 내용이 바뀐 경우에만 교체 + README 행 갱신
install_doc() {
  local src="$1" name="$2" ver="$3"
  if [[ -f "$OUT/$name" ]] && cmp -s "$src" "$OUT/$name"; then
    echo "  $name: 변경 없음"
    return 0
  fi
  mv "$src" "$OUT/$name"
  update_readme_row "$name" "$ver"
  echo "  $name: 갱신 (version=$ver)"
}

# 3), 4)
install_doc "$TMP/openapi.json"     "openapi.json"     "$OPENAPI_VER"
install_doc "$TMP/asyncapi.json"    "asyncapi.json"    "$ASYNCAPI_VER"
install_doc "$TMP/overview.md"      "overview.md"      "-"
install_doc "$TMP/api-reference.md" "api-reference.md" "-"

echo "done: openapi=$OPENAPI_VER asyncapi=$ASYNCAPI_VER → $OUT"
