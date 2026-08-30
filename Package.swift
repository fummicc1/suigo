// swift-tools-version: 6.0
import PackageDescription

// 2 つの製品に分かれている。
//
//   Suigo          インタープリタ本体 + dev トランスポート（SocketTransport は DEBUG のみ）+ MockTransport。
//                  Go の c-archive を必要としないため、単体テストや dev 専用のホストはこれだけで完結する。
//   SuigoEmbedded  本番経路。EmbeddedTransport と SuigoView.standard() を提供し、CSuigo（手書きヘッダ）に依存する。
//                  これをリンクするアプリは scripts/build-xcframework.sh で作った libsuigo.a も一緒にリンクする。
//                  SwiftPM のパッケージにはバイナリを含めない。
let package = Package(
    name: "Suigo",
    platforms: [.iOS(.v17)],
    products: [
        .library(name: "Suigo", targets: ["Suigo"]),
        .library(name: "SuigoEmbedded", targets: ["SuigoEmbedded"]),
    ],
    targets: [
        .target(
            name: "Suigo",
            path: "Sources/Suigo",
            swiftSettings: [.swiftLanguageMode(.v6)]
        ),
        .target(
            name: "CSuigo",
            path: "Sources/CSuigo",
            publicHeadersPath: "include",
            // Go ランタイムが必要とするリンク（検証C で判明）。
            linkerSettings: [
                .linkedLibrary("resolv"),
                .linkedFramework("CoreFoundation"),
            ]
        ),
        .target(
            name: "SuigoEmbedded",
            dependencies: ["Suigo", "CSuigo"],
            path: "Sources/SuigoEmbedded",
            swiftSettings: [.swiftLanguageMode(.v6)]
        ),
        .testTarget(
            name: "SuigoTests",
            dependencies: ["Suigo"],
            path: "Tests/SuigoTests",
            // wire は ../../testdata/wire へのシンボリックリンク。Go 側と同じフィクスチャを読む。
            resources: [.copy("wire")],
            swiftSettings: [.swiftLanguageMode(.v6)]
        ),
    ]
)
