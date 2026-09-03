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
| `openapi.json` | OpenAPI 3.1. REST 엔드포인트(1.2.14 기준 36 operations, 13 tags, x-tagGroups 5개), 스키마, 요청/응답 예시, 인증, 에러, rate limit 의 정본 |
| `asyncapi.json` | AsyncAPI 3.0. 웹소켓 채널(connection, realtime-trade, realtime-orderbook, realtime-order), 구독 선언, 메시지 스키마, 연결 제한, keepalive, 재연결의 정본 |
| `overview.md` | 사람이 읽는 개요와 운영 규칙 — 시작하기(curl 예시), Rate Limits(응답 헤더·429 대응), 에러 응답, 웹소켓 연동 절차 |
| `api-reference.md` | 엔드포인트·모델 한눈에 보기 표(openapi-generator 스타일). 링크는 토스 서버의 `Apis/*.md`, `Models/*.md` 로 연결되며 그 파일들은 `openapi.json` 과 중복이라 보관하지 않음 |

JSON 두 파일은 원본 포맷과 무관하게 항상 `jq .` 로 정규화(pretty-print)해 저장합니다. git diff 가 가능하고 토스 측 포맷 변경에 영향받지 않도록 하기 위함입니다.

## 서버 / 인증 요약

- REST: `https://openapi.tossinvest.com`
- WebSocket: `wss://openapi-ws.tossinvest.com/ws/v1`
- 인증: OAuth 2.0 Client Credentials Grant 로 access token 발급(`POST /oauth2/token`). 토큰 발급을
  제외한 모든 API 는 `Authorization: Bearer {access_token}` 헤더 사용.
- 계좌·자산·주문·조건주문 API 는 추가로 `X-Tossinvest-Account: {accountSeq}` 헤더 필요.
  accountSeq 는 `GET /api/v1/accounts` 로 조회하며, 이 호출 자체는 헤더가 필요 없다.
- 웹소켓 핸드셰이크도 같은 access token 을 `Authorization: Bearer` 헤더로 전달.

자세한 내용은 `overview.md` 와 `openapi.json` 의 `components.securitySchemes` 참고.

## 갱신 방법

```bash
./scripts/fetch-docs.sh
```

원본 4개를 다시 받아 내용이 바뀐 파일만 교체하고 위 버전 표를 갱신합니다. `curl`, `jq` 필요.
