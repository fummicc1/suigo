#!/usr/bin/env bash
# サンプルアプリを起動中のシミュレータへビルド・インストール・起動する。
#
#   scripts/run-example.sh              embedded で起動（Go がアプリに入っている本番経路）
#   scripts/run-example.sh 7391         dev で起動（`suigo dev -port 7391` に接続する）
#
# ホストアプリは 1 つで、接続先だけが変わる。SUIGO_DEV_PORT があれば dev、無ければ embedded
# （Sources/SuigoEmbedded/Standard.swift の SuigoView.standard()）。
set -euo pipefail

PORT="${1:-}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
BUNDLE_ID=dev.suigo.example.counter
APP_NAME=SuigoCounter

UDID="$(xcrun simctl list devices booted -j | jq -r '[.devices[][]][0].udid // empty')"
if [[ -z "$UDID" ]]; then
  echo "起動中のシミュレータがありません。Simulator.app を開くか、次を実行してください:" >&2
  echo "  xcrun simctl boot 'iPhone 17 Pro'" >&2
  exit 1
fi
echo "==> simulator: $UDID"

# embedded 経路には Go の c-archive が要る。dev 経路でも project.yml がリンクするので常に必要。
if [[ ! -d "$ROOT/build/Suigo.xcframework" ]]; then
  echo "==> building Suigo.xcframework (初回は数分かかります)"
  "$ROOT/scripts/build-xcframework.sh" ./examples/counter/embedded build
fi

echo "==> generating Xcode project"
( cd "$ROOT/examples/counter" && xcodegen generate >/dev/null )

echo "==> building app"
# DerivedData はリポジトリの外へ置く。iCloud Drive 上だとファイルプロバイダが拡張属性を付け、
# コード署名が "resource fork, Finder information, or similar detritus not allowed" で落ちる。
DD="${SUIGO_DERIVED_DATA:-$HOME/Library/Developer/Xcode/DerivedData/suigo-example}"
xcodebuild build \
  -project "$ROOT/examples/counter/HostApp.xcodeproj" \
  -scheme "$APP_NAME" \
  -destination "platform=iOS Simulator,id=$UDID" \
  -derivedDataPath "$DD" \
  -quiet

APP="$(find "$DD/Build/Products" -name "$APP_NAME.app" -maxdepth 2 | head -1)"
echo "==> installing $APP"
xcrun simctl install "$UDID" "$APP"
xcrun simctl terminate "$UDID" "$BUNDLE_ID" 2>/dev/null || true

if [[ -n "$PORT" ]]; then
  echo "==> launching in DEV mode (connecting to suigo dev on 127.0.0.1:$PORT)"
  SIMCTL_CHILD_SUIGO_DEV_PORT="$PORT" xcrun simctl launch "$UDID" "$BUNDLE_ID"
else
  echo "==> launching in EMBEDDED mode (Go runs inside the app)"
  xcrun simctl launch "$UDID" "$BUNDLE_ID"
fi
open -a Simulator
