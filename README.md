# suigo

**Write your iOS UI in Go. Render it as real SwiftUI.**

suigo is an in-process, server-driven UI framework: Go builds a view tree, a thin Swift interpreter turns it into native SwiftUI views, and events flow back to Go. The Go side runs either as a separate process during development (**hot reload in under a second, state preserved**) or linked into the app as a C archive for release.

> **Status: v0.1 — a working, measured prototype built as a learning project.**
> Counter and TODO samples run on the iOS Simulator over both transports and, checked by hand at commit `b1d2647`, on a physical iPhone 15 Pro (iOS 27) over the embedded transport; hot reload is verified end-to-end on the simulator.
> This is not a production framework. Read *Scope* and *Known limitations* before depending on it.

## Why this exists

"Write iOS UI in Go" is not new — [Fyne](https://github.com/fyne-io/fyne) and [Gio](https://gioui.org/) already do it. They draw everything themselves, so the result is not native SwiftUI.

The gap suigo fills is exactly one point: **the UI you write in Go is rendered as genuine SwiftUI views** — `Text`, `Button`, `List`, `Toggle`, `TextField` — with native look, accessibility, and OS behavior. Nothing else about suigo is unique; only this.

The second thing suigo gives you is a development loop that SwiftUI Previews cannot: Previews are fast but run on mock data. `suigo dev` runs your **real Go logic with real state** and reflects a saved file in the simulator in ~0.9 s (p50), keeping your model across the reload.

## How it works

```
   Development (dev transport)                 Release (embedded transport)
   ───────────────────────────                 ──────────────────────────────
   Go source ──fsnotify──▶ suigo dev           iOS app (single process)
                             │ go build         ┌──────────────┐  suigo_send   ┌────────────┐
                             ▼                  │ Swift        │──────────────▶│ Go         │
                        Go child process        │ interpreter  │               │ (libsuigo.a│
                             │ stdio            │              │◀──────────────│  c-archive)│
                             ▼                  │ reader thread│ suigo_next_   └────────────┘
   iOS Simulator ◀────TCP (stays connected)     └──────────────┘   frame (blocking)
```

Both transports carry the **same JSON frames**. The interpreter does not know which one it is on.

- **Go** owns the model. `View(model) Node` is a pure function; `Store.Update` queues a mutation and the render loop applies it before the next frame (a single goroutine touches the model, so there is no data race by construction).
- **Wire** is JSON. The full tree is sent on every render; SwiftUI's own `ForEach` identity diffing plus `Equatable` node values do the rest (measured and guarded by a regression test: a one-node change in a 511-node tree re-evaluates 3 view bodies).
- **Swift** is one `switch` from node type to SwiftUI view. Unknown node types render as `EmptyView` and log; unknown modifiers are ignored. Adding a node type never crashes an older host.
- **Boundary** (release) is bytes only: Go mallocs a frame, Swift copies and frees it; Swift → Go is copied during the call. No function pointers cross the boundary, so Swift 6 strict concurrency has nothing to complain about.

The wire contract is frozen in [`docs/wire-format.md`](docs/wire-format.md). Go and Swift share the fixtures in `testdata/wire/`.

## Quick start

Requirements: Go 1.26+, Xcode 26 / Swift 6.2+, iOS 17+ target. Only the iOS Simulator is supported for hot reload in v0.1.

### 1. Write the UI in Go

```go
package ui

type Model struct{ Count int }

func init() { suigo.Register(Model{}, View) }

func View(m Model, s *suigo.Store[Model]) suigo.Node {
    return suigo.VStack(
        suigo.Text(strconv.Itoa(m.Count)).Font(suigo.LargeTitle).ForegroundColor(suigo.Blue),
        suigo.Button("+", func() { s.Update(func(m *Model) { m.Count++ }) }).Padding(12),
    ).Spacing(16)
}
```

### 2. Hot reload during development

```sh
# terminal 1 — supervises the Go process and proxies to the simulator
go run ./cmd/suigo dev -port 7391 ./examples/counter/dev

# terminal 2 — build, install and launch the sample against it
scripts/run-example.sh 7391
```

Now edit `examples/counter/ui/counter.go` and save. The simulator updates without relaunching and keeps its counter value.

In your own app, put `SuigoView.standard()` in the `WindowGroup` and set `SUIGO_DEV_PORT` in the Xcode scheme's environment (Debug only).

### 3. Ship it

```sh
scripts/build-xcframework.sh ./examples/counter/embedded build
```

Link `build/Suigo.xcframework` and the `SuigoEmbedded` SwiftPM product. The same `SuigoView.standard()` now talks to the Go runtime inside the app — `scripts/run-example.sh` with no port argument launches the sample this way. The xcframework carries a device slice (arm64) and a universal simulator slice (arm64 + x86_64; the latter runs under Rosetta). Release builds contain no TCP code, verified by `scripts/check-release-symbols.sh`.

See `examples/counter` (xcodegen `project.yml` + XCUITests) and `examples/todo` (List/ForEach identity, and one TextField per commit policy: a blur-committed title, a submit-committed input and a debounce-committed filter).

## What is in v0.1

| Area | Contents |
|---|---|
| Nodes (14) | Text, Button, VStack, HStack, ZStack, Spacer, Divider, ScrollView, List, ForEach, TextField, Toggle, ProgressView, Image (SF Symbol / bundle asset) |
| Modifiers | padding, font, foregroundColor, background, frame, cornerRadius, spacing, disabled, hidden, id |
| State | `Store[M]` with queued mutations; snapshot/restore across hot reloads via JSON |
| Transports | `suigo dev` (TCP, Debug only, localhost only) and embedded (C ABI, c-archive) |
| Tooling | `suigo dev` with fsnotify, rebuild, state hand-off, build-failure overlay, `--metrics`; xcframework build script; release symbol guard |
| Tests | Go: 80+ unit tests under `-race` (runs on Linux, no macOS needed); Swift: 43 unit tests against shared fixtures incl. performance regression (body re-evaluation count, decode budget); XCUITests for both samples over both transports |

## Measured numbers (iPhone 17 Pro Max simulator, iOS 26.5, Apple Silicon)

| What | Result |
|---|---|
| Boundary cost (Go encode + Swift decode), 31 / 91 / 511 / 1000 nodes | 0.3 / 1.1 / 5.2 / 10.3 ms |
| Dominant cost | Swift `JSONDecoder` (~8× the Go side) |
| Re-render after a 1-node change, any tree size up to 511 | 3 view bodies, 0.8–3.8 ms |
| Re-render when **every** node changes, 511 nodes | 24–28 ms (over the 16.7 ms frame budget) |
| Practical tree size | **~300 nodes** per screen |
| Hot reload, save → screen (p50 / max, 8 runs) | **877 ms / 1011 ms** (warm `go build` ≈ 0.3 s) |
| C ABI leak check, 1000 frames of 32 KB | +1 MB with `suigo_free`, +48 MB without (the check works) |
| App size added by the Go runtime + counter UI (Release, stripped, simulator arm64) | **0.4 MB → 2.9 MB (+2.6 MB)**; Debug bundle +2.8 MB |

## Scope — what suigo deliberately does not do

- **Not a SwiftUI API surface.** 14 node types. No NavigationStack, Picker, Sheet, Alert, Slider, Stepper, TabView, LazyVGrid (candidates for v0.2). Requests to "just add X" will be weighed against this list on purpose.
- No rich animation or custom drawing.
- No embedding into existing UIKit apps; v0.1 assumes a fresh SwiftUI host.
- No remote image URLs or image bytes over the wire.
- No Android. The boundary is designed so a Compose interpreter could be added, but none exists.
- No Go-side diff engine. Full-tree send is measured to be sufficient at this scale; a diff will only be built if measurement says so.

## Known limitations

- **`golang/go#57651`** (open since 2023-01): iOS apps containing Go ≥ 1.18 can drop to ~5 fps **while a physical device is attached to Xcode** (not when detached, not with Instruments running). This affects development, not shipped apps. suigo's dev transport keeps Go out of the app process entirely during development, so the condition does not arise there. With the c-archive setup used here it has **not been reproduced so far**: in one manual run of both samples on an iPhone 15 Pro (iOS 27) at commit `b1d2647`, launched from Xcode, a 60 fps screen recording showed 46–54 changed frames per second during toggle and delete animations in the TODO sample, nowhere near 5 fps. That is a single run and the attached state was not logged, so read it as "not observed", not "cannot happen".
- **Practical limit ≈ 300 nodes.** Beyond that, a full-tree change exceeds the frame budget. Split screens or wait for v0.2.
- **`suigo dev` has no authentication.** It listens on `127.0.0.1` only and is compiled out of Release. Do not expose it on a LAN; a device-side hot reload will need auth first.
- **Hot-reload state hand-off is JSON.** Unexported fields are dropped; a renamed field falls back to its zero value with a warning overlay.
- **Simulator only for hot reload** in v0.1.
- **Building inside iCloud Drive breaks code signing** (`resource fork, Finder information, or similar detritus not allowed`). Keep DerivedData outside the synced folder; `scripts/run-example.sh` already does.

## Compared to

Three questions separate these: is the **UI written in Go**, does it **render as native SwiftUI** (rather than pixels the framework draws itself), and can you **hot reload against real state** (rather than mock data in a preview)?

| | UI written in Go | Renders as native SwiftUI | Hot reload against real state | Notes |
|---|---|---|---|---|
| **suigo** | yes | yes | yes | 14 node types, prototype |
| Fyne / Gio | yes | no — draws its own widgets | – | mature, cross-platform |
| `gomobile build` (x/mobile/app) | yes | no — draws its own widgets | – | the Go wiki calls it "more of a curio" |
| Govinci | yes | no iOS renderer yet (UIKit planned) | – | JSON tree, Android only |
| Flutter + Go via FFI or `gomobile bind` | no — logic only | no — draws its own widgets | yes | mature |
| Go core + hand-written SwiftUI (the Tailscale way) | no — logic only | yes | previews only, on mock data | what suigo tries to replace |
| Kotlin Multiplatform + Compose | no — Go can't be reused | no | – | |
| Plain SwiftUI | no | yes | previews only, on mock data | often the right answer |

If your UI is simple and you don't have a Go core to reuse, plain SwiftUI is the better tool.

**A note on gomobile.** suigo does not use it. The name covers two different things: `gomobile bind` generates Swift/Java bindings so native code can call into Go — the UI stays in Swift; `gomobile build` produces an all-Go app that draws its own widgets. suigo wants neither. It builds the Go side with `go build -buildmode=c-archive` and ships a hand-written C header, so the only thing crossing the boundary is a byte buffer — which is what keeps the door open for a Compose interpreter later. Incidentally, `golang/go#57651` was reported against `gomobile bind`; a single manual run with the plain c-archive used here did not show it (see *Known limitations*).

## Reference implementations this builds on

- [tailscale/libtailscale](https://github.com/tailscale/libtailscale) — c-archive → xcframework build.
- [bipa-app/swiftui-json-render](https://github.com/bipa-app/swiftui-json-render) — JSON → SwiftUI interpretation.
- [grahms/govinci](https://github.com/grahms/govinci) — Go declarative DSL with a JSON tree.
- [youngdynasty/go-swift](https://github.com/youngdynasty/go-swift) — cgo embedding in a Swift app.

Design notes (Japanese) live in `docs/`. The wire contract in `docs/wire-format.md` is the source of truth for both sides.

## License

Apache-2.0. No CLA.
