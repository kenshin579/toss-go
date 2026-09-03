# 토스증권 Open API 문서 카탈로그 설계

- 작성일: 2026-09-04
- 상태: 확정 (브레인스토밍 완료)
- 레포: `github.com/kenshin579/toss-go` (워크스페이스 `toss-go/`, branch `chore/api-docs`)
- 토픽: 토스증권 Open API 공식 문서를 SDK 개발의 1차 참조로 레포에 보관

## 배경 / 목적

`toss-go`(토스증권 Open API Go 클라이언트)를 만들기에 앞서 API 문서를 레포에 갖춘다.
fmp-go 는 문서 사이트가 JS 렌더링 전용이라 Playwright 크롤러로 274 페이지를 md 로 변환해야
했다. 토스는 상황이 다르다 — **AI/기계 판독용 원본을 공식 제공**한다. 따라서 우리가 md 를
새로 생성하지 않고, 공식 원본을 그대로 보관해 SDK 개발의 source of truth 로 쓴다.

## 사전 조사 결과 (확정 사실, 2026-09-04 기준)

- `https://developers.tossinvest.com/docs` 는 Scalar 기반 JS 렌더링 페이지. SSR HTML 에는
  내용이 없고 "For AI agents and non-JavaScript fetchers, use: /llms.txt" 안내만 있다.
- `https://developers.tossinvest.com/llms.txt` 가 아래 4개를 **Source of Truth** 로 안내한다.

| 원본 URL | 내용 | 버전 |
|---|---|---|
| `https://openapi.tossinvest.com/openapi-docs/latest/openapi.json` | OpenAPI 3.1. REST 36 operations, 13 tags, x-tagGroups 5개(인증/시세·종목 정보/계좌·자산/주문/조건주문), components.schemas 90개, 응답 example 35개 포함 | 1.2.14 |
| `https://openapi.tossinvest.com/openapi-docs/latest/asyncapi.json` | AsyncAPI 3.0. 웹소켓 채널 4개(connection, realtime-trade, realtime-orderbook, realtime-order), operations 10개 | 1.2.2 |
| `https://openapi.tossinvest.com/openapi-docs/overview.md` | 개요, 시작하기(curl 예시), Rate Limits(응답 헤더·429 대응), 에러 응답, 웹소켓 연동(연결·구독·PING·전달 보장·에러) | - |
| `https://openapi.tossinvest.com/openapi-docs/latest/api-reference/README.md` | openapi-generator 스타일. 엔드포인트 표(Class/Method/HTTP/설명) + 모델 목록 + 인증 요약. 하위 `Apis/*.md`(13) `Models/*.md`(124) 로 링크 | - |

- 서버: REST `https://openapi.tossinvest.com`, WebSocket `wss://openapi-ws.tossinvest.com/ws/v1`.
- 인증: OAuth 2.0 Client Credentials Grant 로 access token 발급. 계좌·자산·주문·조건주문 API 는
  `Authorization: Bearer` 에 더해 `X-Tossinvest-Account` 헤더 필요. WS 핸드셰이크도 같은
  토큰을 `Authorization: Bearer` 헤더로 전달. (`GET /api/v1/accounts` 자체는 이 헤더가 필요 없음.)
- 원본 JSON 포맷은 파일마다 다르다 — openapi.json(418KB)은 이미 pretty-print 되어 제공되고 asyncapi.json 은 한 줄(minified)이다. 우리는 둘 다 `jq .` 로 정규화해 저장한다.
- `Apis/*.md` / `Models/*.md` 는 `openapi.json` 과 내용이 완전히 중복되고 응답 예시가 없다.

## 결정 사항 (브레인스토밍)

1. **md 생성 안 함.** 토스 공식 원본을 그대로 보관한다. (검토했던 대안: 토스 md 137개 전부
   미러링 / openapi.json 에서 fmp-go 식 엔드포인트별 md 생성 — 둘 다 원본 대비 가치가 없어
   기각.)
2. **보관 대상 4개**: `openapi.json`, `asyncapi.json`, `overview.md`, `api-reference/README.md`.
   `Apis/*.md`, `Models/*.md` 는 보관하지 않는다(중복).
3. 우리가 쓰는 파일은 `docs/api/README.md`(안내) 와 `scripts/fetch-docs.sh`(갱신) 둘뿐.
4. JSON 은 **pretty-print 해서 저장**한다. 토스가 버전을 올렸을 때 git diff 로 변경점을 볼 수
   있어야 하기 때문.

## 파일 구성

```
toss-go/
├── docs/api/
│   ├── README.md          # 우리가 작성: 출처·버전·갱신법·파일 안내
│   ├── openapi.json       # 토스 원본 (REST, OpenAPI 3.1) — jq pretty-print
│   ├── asyncapi.json      # 토스 원본 (WebSocket, AsyncAPI 3.0) — jq pretty-print
│   ├── overview.md        # 토스 원본 (개요·인증·Rate Limit·에러·WS 연동)
│   └── api-reference.md   # 토스 원본 api-reference/README.md (엔드포인트·모델 표)
└── scripts/
    └── fetch-docs.sh      # 원본 4개 재다운로드 + README 버전 표 갱신
```

## `scripts/fetch-docs.sh`

- bash, `set -euo pipefail`. 의존: `curl`, `jq`.
- 레포 루트 기준 경로를 스크립트 위치에서 계산(`$(dirname "$0")/..`)해 어디서 실행해도 동작.
- 4개 URL 을 `curl -fsSL` 로 받는다. HTTP 실패 시 즉시 중단(부분 갱신 방지를 위해 임시 파일에
  받은 뒤 모두 성공했을 때 최종 경로로 이동).
- `openapi.json`, `asyncapi.json` 은 `jq .` 로 pretty-print 해 저장(키 정렬 없음 — 원본 순서
  유지). `overview.md`, `api-reference.md` 는 그대로 저장.
- 다운로드 후 `jq -r .info.version` 으로 두 JSON 의 버전을 읽어 콘솔에 출력하고,
  `docs/api/README.md` 의 버전 표 행(`| openapi.json | ... |`, `| asyncapi.json | ... |`)과
  가져온 날짜를 `sed` 로 갱신한다.
- 토스 문서 안의 절대 URL 링크는 손대지 않는다. 미보관 파일(`Apis/*.md` 등)로 가는 링크도
  토스 서버에서 그대로 열리므로 문제 없다.
- 멱등: 원본이 바뀌지 않았으면 재실행해도 diff 가 없다.

## `docs/api/README.md` 내용

1. 제목 + 한 줄 설명(토스증권 Open API 공식 문서 보관본, 우리가 생성한 것이 아님).
2. 출처: `llms.txt` URL 과 원본 4개 URL.
3. 가져온 버전 표: 파일 / 버전 / 가져온 날짜 (fetch-docs.sh 가 갱신).
4. 파일 안내: 각 파일이 무엇의 source of truth 인지 한 줄씩.
   - `openapi.json` — REST 엔드포인트·스키마·요청/응답 예시·인증·에러·rate limit 의 정본.
   - `asyncapi.json` — WS 채널·구독 선언·메시지 스키마·연결 제한·keepalive·재연결의 정본.
   - `overview.md` — 사람이 읽는 개요와 운영 규칙(Rate Limit, 에러 모델, WS 연동 절차).
   - `api-reference.md` — 엔드포인트·모델 한눈에 보기 표.
5. 서버 URL 과 인증 요약(OAuth2 Client Credentials, 계좌 API 는 `X-Tossinvest-Account`).
6. 갱신 방법: `./scripts/fetch-docs.sh`.

톤은 fmp-go / ecos-go 의 `docs/api/README.md` 를 따른다(한국어, 표 중심).

## 검증

- `./scripts/fetch-docs.sh` 실행 후 `docs/api/` 에 5개 파일 존재.
- `jq empty docs/api/openapi.json docs/api/asyncapi.json` 통과.
- `jq '[.paths[] | to_entries[] | select(.key|test("^(get|post|put|delete|patch)$"))] | length' docs/api/openapi.json` 이 36.
- `jq '.channels|keys|length' docs/api/asyncapi.json` 이 4.
- README 버전 표에 1.2.14 / 1.2.2 와 오늘 날짜가 채워짐.
- 스크립트를 두 번 실행해 `git status` 에 변경이 없음(멱등).

## 범위 밖 / 후속

- SDK 클라이언트 설계·구현(패키지 구조, 토큰 발급·캐시, REST 그룹, WS 클라이언트) — 별도
  브레인스토밍.
- `Apis/*.md`, `Models/*.md` 미보관. alpha/qa 서버(`openapi-alpha`, `openapi-qa`) 문서 미대상.
- 갱신 자동화(CI/cron) — 수동 실행으로 충분.
