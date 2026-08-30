#!/usr/bin/env bash
# Release ビルドに dev トランスポートが混入していないことを機械的に確認する（AC-8 / RISK-4 / TX-001, TX-002）。
#
#   scripts/check-release-symbols.sh <Swift の Release バイナリ> [<Go の c-archive>]
#
# Go 側は //go:build suigodev、Swift 側は #if DEBUG で隔離しているが、
# 「隔離したつもり」を CI で検査しなければ RISK-4 は本番に出る（design §10-9）。
set -euo pipefail

SWIFT_BIN="${1:?usage: $0 <swift release binary> [go c-archive]}"
GO_ARCHIVE="${2:-}"
fail=0

check() {
  local what="$1" file="$2" pattern="$3"
  if nm -g "$file" 2>/dev/null | grep -qiE "$pattern" || strings "$file" | grep -qiE "$pattern"; then
    echo "✘ $what: found '$pattern' in $file"
    fail=1
  else
    echo "✔ $what: no '$pattern'"
  fi
}

echo "== Swift release binary: $SWIFT_BIN"
check "SocketTransport type"   "$SWIFT_BIN" "SocketTransport"
check "dev port env lookup"    "$SWIFT_BIN" "SUIGO_DEV_PORT"
check "NWConnection usage"     "$SWIFT_BIN" "NWConnection"

if [[ -n "$GO_ARCHIVE" ]]; then
  echo "== Go c-archive: $GO_ARCHIVE"
  check "devserver package"    "$GO_ARCHIVE" "suigo/devserver"
  check "SetDevServe hook"     "$GO_ARCHIVE" "SetDevServe"
  check "net.Listen"           "$GO_ARCHIVE" "net\.\(\*ListenConfig\)\.Listen|net\.Listen"
fi

if [[ $fail -ne 0 ]]; then
  echo "FAIL: dev transport leaked into a release artifact"
  exit 1
fi
echo "PASS: release artifacts are free of dev transport code"
