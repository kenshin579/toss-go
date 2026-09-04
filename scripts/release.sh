#!/usr/bin/env bash
# release.sh — toss-go 릴리스 자동화
#
#   ./scripts/release.sh vX.Y.Z
#
# 동작:
#   1. main 브랜치 + 클린 + origin/main 동기화 확인
#   2. build / vet / test
#   3. 모듈 zip 패키징 검증 (잘못된 파일명 등으로 `go get` 이 깨지는 것을 사전 차단)
#   4. 태그 생성 + 푸시
#   5. GitHub Release 생성 (gh release create --generate-notes)
set -euo pipefail

VERSION="${1:-}"
if [[ -z "$VERSION" ]]; then
  echo "usage: ./scripts/release.sh vX.Y.Z" >&2
  exit 1
fi
if [[ ! "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "error: version must look like v1.2.3 (got: $VERSION)" >&2
  exit 1
fi

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT"

# --- 1. 사전 상태 검증 -------------------------------------------------------
branch="$(git rev-parse --abbrev-ref HEAD)"
if [[ "$branch" != "main" ]]; then
  echo "error: must release from main (currently on: $branch)" >&2
  exit 1
fi
if [[ -n "$(git status --porcelain)" ]]; then
  echo "error: working tree not clean" >&2
  exit 1
fi
git fetch --tags origin
if [[ "$(git rev-parse HEAD)" != "$(git rev-parse origin/main)" ]]; then
  echo "error: local main is not in sync with origin/main" >&2
  exit 1
fi
if git rev-parse "$VERSION" >/dev/null 2>&1; then
  echo "error: tag $VERSION already exists" >&2
  exit 1
fi

# --- 2. build / vet / test ---------------------------------------------------
echo "==> go build ./..."
go build ./...
echo "==> go vet ./..."
go vet ./...
echo "==> go test ./..."
go test ./...

# --- 3. 모듈 zip 패키징 검증 -------------------------------------------------
# golang.org/x/mod/zip.CheckDir 로 모듈에 담길 파일들을 검사한다.
# 잘못된 파일명(예: 가운뎃점 '·')이 있으면 여기서 실패시켜 깨진 태그 릴리스를 막는다.
# toss-go go.mod 를 오염시키지 않도록 임시 모듈에서 실행한다.
echo "==> module file validation (zip.CheckDir)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
cat > "$tmp/main.go" <<'GO'
package main

import (
	"fmt"
	"os"

	"golang.org/x/mod/zip"
)

func main() {
	cf, _ := zip.CheckDir(os.Args[1])
	if err := cf.Err(); err != nil {
		fmt.Fprintln(os.Stderr, "module file validation failed:")
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("module files OK (%d valid)\n", len(cf.Valid))
}
GO
(
  cd "$tmp"
  go mod init toss-go-release-check >/dev/null
  go get golang.org/x/mod/zip@latest >/dev/null 2>&1
  go run . "$ROOT"
)

# --- 4. 태그 + 푸시 ----------------------------------------------------------
echo "==> tag $VERSION"
git tag "$VERSION"
git push origin "$VERSION"

# --- 5. GitHub Release -------------------------------------------------------
echo "==> gh release create $VERSION"
gh release create "$VERSION" --title "$VERSION" --generate-notes

echo "✅ released $VERSION"
