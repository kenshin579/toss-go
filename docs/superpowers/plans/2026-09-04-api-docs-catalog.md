# 토스증권 Open API 문서 카탈로그 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 토스증권 Open API 의 공식 기계 판독용 문서 4개를 `docs/api/` 에 보관하고, 재다운로드 스크립트와 안내 README 를 갖춘다.

**Architecture:** md 생성기는 만들지 않는다. `llms.txt` 가 안내하는 원본(openapi.json, asyncapi.json, overview.md, api-reference/README.md)을 `scripts/fetch-docs.sh` 가 curl 로 받아 JSON 은 jq 로 pretty-print 해 저장하고, 내용이 바뀐 파일만 교체하며 `docs/api/README.md` 의 버전 표를 갱신한다. 4개 모두 임시 디렉토리에 받은 뒤 반영하므로 부분 갱신이 없다.

**Tech Stack:** bash, curl, jq. Go 코드 없음(SDK 는 후속 스펙).

**Spec:** `docs/superpowers/specs/2026-09-04-api-docs-catalog-design.md`

**Branch:** `chore/api-docs` (이미 생성·스펙 커밋됨)

---

## 파일 구조

| 경로 | 책임 |
|---|---|
| `docs/api/README.md` | 우리가 작성. 출처·버전 표·파일 안내·서버/인증 요약·갱신법 |
| `docs/api/openapi.json` | 토스 원본(REST). 스크립트가 생성 |
| `docs/api/asyncapi.json` | 토스 원본(WebSocket). 스크립트가 생성 |
| `docs/api/overview.md` | 토스 원본(개요·운영 규칙). 스크립트가 생성 |
| `docs/api/api-reference.md` | 토스 원본(엔드포인트·모델 표). 스크립트가 생성 |
| `scripts/fetch-docs.sh` | 원본 4개 다운로드 + README 버전 표 갱신 |

---

### Task 1: `docs/api/README.md` 작성

**Files:**
- Create: `docs/api/README.md`

- [ ] **Step 1: README 작성**

버전 표의 행 형식은 Task 2 의 스크립트가 `sed` 로 찾는 패턴이므로 정확히 `| \`파일명\` | 버전 | 날짜 |` 형태를 지킨다.

```bash
cd /Users/user/src/workspace_moneyflow/toss-go && mkdir -p docs/api && cat > docs/api/README.md << 'EOF'
# 토스증권 Open API 문서

토스증권 Open API(https://developers.tossinvest.com/docs)의 **공식 문서 원본 보관본**입니다.
우리가 생성한 문서가 아니라 토스가 AI/기계 판독용으로 제공하는 파일을 그대로 가져온 것이며,
`toss-go` SDK 개발의 1차 참조(source of truth)로 씁니다.

## 출처

토스는 https://developers.tossinvest.com/llms.txt 에서 아래 4개를 source of truth 로 안내합니다.

| 파일 | 원본 URL |
| --- | --- |
| `openapi.json` | https://openapi.tossinvest.com/openapi-docs/latest/openapi.json |
| `asyncapi.json` | https://openapi.tossinvest.com/openapi-docs/latest/asyncapi.json |
| `overview.md` | https://openapi.tossinvest.com/openapi-docs/overview.md |
| `api-reference.md` | https://openapi.tossinvest.com/openapi-docs/latest/api-reference/README.md |

## 가져온 버전

`./scripts/fetch-docs.sh` 가 갱신합니다. 버전은 JSON 의 `info.version`, md 는 버전 정보가 없어 `-` 입니다.

| 파일 | 버전 | 가져온 날짜 |
| --- | --- | --- |
| `openapi.json` | 1.2.14 | 2026-09-04 |
| `asyncapi.json` | 1.2.2 | 2026-09-04 |
| `overview.md` | - | 2026-09-04 |
| `api-reference.md` | - | 2026-09-04 |

## 파일 안내

| 파일 | 무엇의 정본인가 |
| --- | --- |
| `openapi.json` | OpenAPI 3.1. REST 엔드포인트(36 operations, 13 tags, x-tagGroups 5개), 스키마, 요청/응답 예시, 인증, 에러, rate limit 의 정본 |
| `asyncapi.json` | AsyncAPI 3.0. 웹소켓 채널(connection, realtime-trade, realtime-orderbook, realtime-order), 구독 선언, 메시지 스키마, 연결 제한, keepalive, 재연결의 정본 |
| `overview.md` | 사람이 읽는 개요와 운영 규칙 — 시작하기(curl 예시), Rate Limits(응답 헤더·429 대응), 에러 응답, 웹소켓 연동 절차 |
| `api-reference.md` | 엔드포인트·모델 한눈에 보기 표(openapi-generator 스타일). 링크는 토스 서버의 `Apis/*.md`, `Models/*.md` 로 연결되며 그 파일들은 `openapi.json` 과 중복이라 보관하지 않음 |

JSON 두 파일은 git diff 가 가능하도록 `jq .` 로 pretty-print 해 저장합니다(원본은 한 줄).

## 서버 / 인증 요약

- REST: `https://openapi.tossinvest.com`
- WebSocket: `wss://openapi-ws.tossinvest.com/ws/v1`
- 인증: OAuth 2.0 Client Credentials Grant 로 access token 발급(`POST /oauth2/token`). 모든 API 는
  `Authorization: Bearer {access_token}` 헤더 사용.
- 계좌·자산·주문 API 는 추가로 `X-Tossinvest-Account` 헤더 필요.
- 웹소켓 핸드셰이크도 같은 access token 을 `Authorization: Bearer` 헤더로 전달.

자세한 내용은 `overview.md` 와 `openapi.json` 의 `components.securitySchemes` 참고.

## 갱신 방법

```bash
./scripts/fetch-docs.sh
```

원본 4개를 다시 받아 내용이 바뀐 파일만 교체하고 위 버전 표를 갱신합니다. `curl`, `jq` 필요.
EOF
file -I docs/api/README.md
```

Expected: `docs/api/README.md: text/plain; charset=utf-8`

- [ ] **Step 2: 커밋**

```bash
cd /Users/user/src/workspace_moneyflow/toss-go && git add docs/api/README.md && git commit -m "docs: 토스 Open API 문서 보관 안내 README

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

---

### Task 2: `scripts/fetch-docs.sh` 작성 + 실패/성공/멱등 검증

**Files:**
- Create: `scripts/fetch-docs.sh`

- [ ] **Step 1: 스크립트 작성**

```bash
cd /Users/user/src/workspace_moneyflow/toss-go && mkdir -p scripts && cat > scripts/fetch-docs.sh << 'EOF'
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
EOF
chmod +x scripts/fetch-docs.sh && bash -n scripts/fetch-docs.sh && echo SYNTAX_OK
```

Expected: `SYNTAX_OK`

- [ ] **Step 2: 실패 경로 검증 — 잘못된 base URL 이면 종료코드 ≠ 0 이고 docs/api 는 그대로**

```bash
cd /Users/user/src/workspace_moneyflow/toss-go && TOSS_DOCS_BASE=https://openapi.tossinvest.com/no-such-path ./scripts/fetch-docs.sh; echo "exit=$?"; ls docs/api
```

Expected: `GET https://openapi.tossinvest.com/no-such-path/latest/openapi.json` 출력 후 curl 오류(`curl: (22) The requested URL returned error: 404`), `exit=22`(0 이 아님), `ls docs/api` 는 `README.md` 만 출력.

- [ ] **Step 3: 성공 경로 실행**

```bash
cd /Users/user/src/workspace_moneyflow/toss-go && ./scripts/fetch-docs.sh
```

Expected 출력(순서대로):
```
GET https://openapi.tossinvest.com/openapi-docs/latest/openapi.json
GET https://openapi.tossinvest.com/openapi-docs/latest/asyncapi.json
GET https://openapi.tossinvest.com/openapi-docs/overview.md
GET https://openapi.tossinvest.com/openapi-docs/latest/api-reference/README.md
  openapi.json: 갱신 (version=1.2.14)
  asyncapi.json: 갱신 (version=1.2.2)
  overview.md: 갱신 (version=-)
  api-reference.md: 갱신 (version=-)
done: openapi=1.2.14 asyncapi=1.2.2 → /Users/user/src/workspace_moneyflow/toss-go/docs/api
```
(토스가 버전을 올렸다면 숫자는 다를 수 있음 — 그 경우 README 표와 스펙의 "사전 조사" 표에 새 버전을 기록한다.)

- [ ] **Step 4: 산출물 검증 — 파일 5개, JSON 유효, pretty-print, operations 36, channels 4**

```bash
cd /Users/user/src/workspace_moneyflow/toss-go && ls docs/api && jq empty docs/api/openapi.json docs/api/asyncapi.json && echo JSON_OK && head -c 2 docs/api/openapi.json | od -c | head -1 && echo "operations=$(jq '[.paths[] | to_entries[] | select(.key|test("^(get|post|put|delete|patch)$"))] | length' docs/api/openapi.json)" && echo "channels=$(jq '.channels|keys|length' docs/api/asyncapi.json)" && grep -E '^\| `(openapi|asyncapi)\.json`' docs/api/README.md && file -I docs/api/overview.md docs/api/api-reference.md
```

Expected:
```
README.md  api-reference.md  asyncapi.json  openapi.json  overview.md
JSON_OK
0000000   {  \n
operations=36
channels=4
| `openapi.json` | 1.2.14 | 2026-09-04 |
| `asyncapi.json` | 1.2.2 | 2026-09-04 |
docs/api/overview.md: text/plain; charset=utf-8
docs/api/api-reference.md: text/plain; charset=utf-8
```
(`{  \n` 은 pretty-print 확인. 날짜는 실행일.)

- [ ] **Step 5: 멱등 검증 — 두 번째 실행은 "변경 없음" 4줄, README 도 그대로**

```bash
cd /Users/user/src/workspace_moneyflow/toss-go && cp docs/api/README.md /tmp/readme.before && ./scripts/fetch-docs.sh | grep -c '변경 없음' && cmp /tmp/readme.before docs/api/README.md && echo README_UNCHANGED
```

Expected:
```
4
README_UNCHANGED
```

- [ ] **Step 6: 커밋 (스크립트 + 원본 4개 + 갱신된 README)**

```bash
cd /Users/user/src/workspace_moneyflow/toss-go && git add scripts/fetch-docs.sh docs/api/ && git status --short && git commit -m "docs: 토스 Open API 공식 문서 원본 4개 보관 + fetch-docs.sh

- openapi.json(1.2.14), asyncapi.json(1.2.2), overview.md, api-reference.md
- JSON 은 git diff 를 위해 jq pretty-print
- scripts/fetch-docs.sh: 전부 받은 뒤 변경분만 교체, README 버전 표 갱신

Co-Authored-By: Claude Fable 5.1 <noreply@anthropic.com>"
```

Expected `git status --short`: `A  docs/api/api-reference.md`, `A  docs/api/asyncapi.json`, `A  docs/api/openapi.json`, `A  docs/api/overview.md`, `A  scripts/fetch-docs.sh` (README 는 Task 1 에서 커밋한 값과 동일하면 목록에 없음).

---

### Task 3: PR 생성

**Files:** 없음 (git 작업만)

- [ ] **Step 1: 브랜치 푸시**

```bash
cd /Users/user/src/workspace_moneyflow/toss-go && git push -u origin chore/api-docs
```

Expected: `branch 'chore/api-docs' set up to track 'origin/chore/api-docs'.`

- [ ] **Step 2: PR 생성 (gh + HEREDOC, 리뷰어 지정 금지)**

```bash
cd /Users/user/src/workspace_moneyflow/toss-go && gh pr create --title "docs: 토스증권 Open API 공식 문서 카탈로그 보관 + fetch-docs.sh" --body "$(cat <<'EOF'
## Summary
- 토스가 `llms.txt` 로 안내하는 공식 기계 판독용 원본 4개를 `docs/api/` 에 보관 (md 생성 없음)
  - `openapi.json`(OpenAPI 3.1, v1.2.14, REST 36 operations) / `asyncapi.json`(AsyncAPI 3.0, v1.2.2, WS 채널 4개) — git diff 를 위해 jq pretty-print
  - `overview.md`(개요·Rate Limit·에러·WS 연동) / `api-reference.md`(엔드포인트·모델 표)
- `scripts/fetch-docs.sh`: 4개 전부 받은 뒤 변경분만 교체, `docs/api/README.md` 버전 표 갱신, 멱등
- 설계 스펙 `docs/superpowers/specs/2026-09-04-api-docs-catalog-design.md`, 계획 `docs/superpowers/plans/2026-09-04-api-docs-catalog.md`

## Test plan
- [x] 잘못된 base URL → 종료코드 ≠ 0, `docs/api` 미변경
- [x] `jq empty` 통과, operations 36, channels 4
- [x] 두 번째 실행 시 "변경 없음" 4건, README diff 없음

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

Expected: PR URL 출력 (`https://github.com/kenshin579/toss-go/pull/1`).
