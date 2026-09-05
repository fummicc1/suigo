#!/usr/bin/env bash
# Go を -buildmode=c-archive で iOS device / simulator（arm64 + x86_64）向けにビルドし、xcframework にまとめる。
#
#   scripts/build-xcframework.sh <利用者の main パッケージ> [出力ディレクトリ]
#   例: scripts/build-xcframework.sh ./examples/counter/embedded build
#
# ビルド対象は suigo の embedded パッケージではなく、利用者側の main パッケージである。
# c-archive は main パッケージをビルドするため、./embedded だけでは利用者の View が含まれない
# （design §9.1）。手順は検証C / Task 1.2 で動作確認したものをそのまま使う。
set -euo pipefail

PKG="${1:?usage: $0 <main package> [out dir]}"
OUT="${2:-build}"
MIN_IOS="${MIN_IOS:-17.0}"
HEADER_DIR="$(cd "$(dirname "$0")/.." && pwd)/Sources/CSuigo/include"

build_slice() {
  local sdk="$1" goarch="$2" target="$3" dir="$4"
  local sysroot clang
  sysroot="$(xcrun --sdk "$sdk" --show-sdk-path)"
  clang="$(xcrun --sdk "$sdk" -f clang)"
  mkdir -p "$dir"
  echo "==> $sdk ($target)"
  CGO_ENABLED=1 GOOS=ios GOARCH="$goarch" \
    CC="$clang -target $target -isysroot $sysroot" \
    CGO_CFLAGS="-target $target -isysroot $sysroot" \
    CGO_LDFLAGS="-target $target -isysroot $sysroot" \
    go build -buildmode=c-archive -o "$dir/libsuigo.a" "$PKG"
  # ヘッダと modulemap は SwiftPM の CSuigo ターゲットが持つ。xcframework にも入れると
  # 'redefinition of module CSuigo' で衝突するため、ここではバイナリだけを出す。
  # cgo が生成した libsuigo.h（main の //export のみ）も使わないので消す。
  rm -f "$dir/libsuigo.h"
}

build_slice iphonesimulator arm64 "arm64-apple-ios${MIN_IOS}-simulator"  "$OUT/sim-arm64"
build_slice iphonesimulator amd64 "x86_64-apple-ios${MIN_IOS}-simulator" "$OUT/sim-x86_64"
build_slice iphoneos        arm64 "arm64-apple-ios${MIN_IOS}"            "$OUT/ios-arm64"

# create-xcframework は同じプラットフォームのライブラリを 2 本受け取れないので、
# シミュレータの 2 アーキテクチャは lipo で 1 本の universal archive にしてから渡す。
echo "==> lipo (simulator arm64 + x86_64)"
mkdir -p "$OUT/sim"
lipo -create "$OUT/sim-arm64/libsuigo.a" "$OUT/sim-x86_64/libsuigo.a" -output "$OUT/sim/libsuigo.a"
lipo -info "$OUT/sim/libsuigo.a"

echo "==> xcframework"
rm -rf "$OUT/Suigo.xcframework"
xcodebuild -create-xcframework \
  -library "$OUT/sim/libsuigo.a" \
  -library "$OUT/ios-arm64/libsuigo.a" \
  -output "$OUT/Suigo.xcframework"

echo "==> done: $OUT/Suigo.xcframework"
ls "$OUT/Suigo.xcframework"
