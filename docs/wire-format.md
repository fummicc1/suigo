# suigo ワイヤ形式 v0.1

- 確定日: 2026-08-30（Task 1.3）
- status: **凍結**。変更するときは Go 側と Swift 側の両方を同時に直すこと

この文書は Go 側と Swift 側の**契約**である。両者はこれだけを見て独立に実装できる。
フィクスチャは `testdata/wire/` にある。

---

## 1. フレーミング

| トランスポート | 枠づけ |
|---|---|
| dev（TCP / stdio） | **4 バイトのリトルエンディアン符号なし長さ** + その長さぶんの JSON |
| embedded（C ABI） | 枠づけ無し。`suigo_next_frame` がバッファ長を返す |

- 長さ 0 は不正とする
- **長さの上限は 16 MB**（`16777216`）。超えたら接続を切る
- JSON は UTF-8。改行は含めても含めなくてもよい

## 2. フレーム

### 2.1 Go → ホスト

```jsonc
{ "type": "render", "seq": 42, "tree": { /* Node */ } }

{ "type": "error", "severity": "build" | "runtime" | "warning",
  "code": "duplicate_id", "message": "..." }

{ "type": "snapshot_result", "model": { /* 任意 */ } }        // dev のみ
{ "type": "snapshot_result", "error": "..." }                 // 失敗時
```

- `seq` は 1 から始まる単調増加。**ホストは自分が見た最大 `seq` 未満のフレームを捨てる**（FR-4.5）
- `code` は機械可読な識別子。テストはメッセージ本文ではなく `code` で判定する

**`code` の一覧（v0.1）**

| code | severity | 意味 |
|---|---|---|
| `build_failed` | build | `go build` が失敗した |
| `duplicate_id` | runtime | 同一ツリー内で ID が重複した |
| `invalid_id` | runtime | 明示 ID に `/` が含まれる |
| `handler_panic` | runtime | イベントハンドラが panic した |
| `node_count_warning` | warning | 描画ツリーが 300 ノードを超えた |
| `restore_failed` | warning | ホットリロード時の状態復元に失敗した |

### 2.2 ホスト → Go

```jsonc
{ "type": "mount", "restore": { /* 任意 */ } }   // restore は dev のみ・省略可
{ "type": "event", "nodeID": "0/2", "kind": "tap", "payload": { "t": "none" } }
{ "type": "env", "colorScheme": "dark",
  "safeArea": { "top": 59, "bottom": 34, "leading": 0, "trailing": 0 },
  "sizeClass": { "h": "compact", "v": "regular" } }
{ "type": "snapshot" }                            // dev のみ
```

### 2.3 未知のフレーム型

**受信側は未知の `type` を無視してログに出す。** クラッシュも切断もしない。

これは未知ノード型・未知修飾子と同じ方針である（FR-4.2 / FR-4.3 / NFR-7）。
この規則があるため、プロトコルに版番号は持たせない。新しいフレーム型を足しても
古い実装は黙って無視するだけで壊れない。

---

## 3. ノード

```jsonc
{
  "id": "0/2",
  "type": "Text",
  "props": { "text": "Hello" },
  "mods": [ { "k": "font", "v": "title" }, { "k": "padding", "v": 8 } ],
  "children": []
}
```

| キー | 必須 | 備考 |
|---|---|---|
| `id` | ✅ | 同一ツリー内で一意。Go 側が採番時に保証する |
| `type` | ✅ | 下表の 14 種のいずれか。未知なら Swift は空 View + ログ |
| `props` | — | **空なら省略する。** 受信側は欠落を空として扱う |
| `mods` | — | **空なら省略する。順序に意味がある** |
| `children` | — | **空なら省略する** |

### 3.1 `props` の型は `(type, key)` で決まる

タグ付き共用体にはしない。値の型が組で一意に決まるためである
（design §0-3。デコードが支配的コストなので JSON を膨らませない）。

| `type` | `props` | 型 | 省略時 |
|---|---|---|---|
| `Text` | `text` | string | `""` |
| `Button` | `label` | string | `""` |
| `TextField` | `label` | string | `""` |
| | `value` | string | `""` |
| | `commit` | string（§3.3） | `"submit"` |
| `Toggle` | `label` | string | `""` |
| | `on` | bool | `false` |
| `Image` | `symbol` | string（SF Symbol 名） | — |
| | `asset` | string（bundle アセット名） | — |
| `ProgressView` | `value` | double（0.0〜1.0） | 省略時は不定形 |
| `VStack` `HStack` `ZStack` | `alignment` | string（§3.4） | `"center"` |
| `ScrollView` | `axis` | string（`"h"` のみ。縦は省略） | `"v"` |
| `List` `ForEach` `Spacer` `Divider` | — | — | — |

`Image` は `symbol` と `asset` のどちらか一方を持つ。両方あれば `symbol` を優先する。
リモート URL と画像バイト列は v0.1 では扱わない（FR-1.7）。

### 3.2 ノード型の一覧（14 種）

**受入基準に含む（11）**: `Text` `Button` `VStack` `HStack` `Spacer` `Divider`
`ScrollView` `List` `ForEach` `TextField` `Toggle`

**含めるが受入基準外（3）**: `ZStack` `ProgressView` `Image`

**v0.2 送り**: `NavigationStack` `Picker` `Sheet` `Alert` `Slider` `Stepper` `TabView` `LazyVGrid`

### 3.3 `TextField.commit` の文法

```
commit := "submit" | "blur" | "debounce:" <正の整数（ミリ秒）>
```

例: `"submit"` / `"blur"` / `"debounce:300"`

解釈できない値は `"submit"` として扱い、ログに出す。

### 3.4 `alignment` の値

- `VStack` / スタック共通: `"leading"` `"center"` `"trailing"`
- `HStack`: `"top"` `"center"` `"bottom"`
- `ZStack` と `frame`: 上記に加え `"topLeading"` `"topTrailing"` `"bottomLeading"` `"bottomTrailing"`

---

## 4. 修飾子

`mods` は**配列であり、順序に意味がある**。Go 側のメソッドチェーン順をそのまま適用する。
`padding` → `background` と `background` → `padding` は SwiftUI 上で結果が異なるため、
受信側はソートも正規化もしてはならない。

| `k` | `v` の型 | 例 |
|---|---|---|
| `padding` | double、または `{top?, bottom?, leading?, trailing?}` | `8` / `{"top":8,"leading":4}` |
| `font` | string（§4.1） | `"title"` |
| `foregroundColor` | string（§4.2） | `"blue"` / `"#FF8800"` |
| `background` | string（§4.2） | `"gray"` |
| `frame` | `{width?, height?, maxWidth?, maxHeight?, alignment?}` | `{"maxWidth":"infinity"}` |
| `cornerRadius` | double | `12` |
| `spacing` | double | `12` |
| `disabled` | bool | `true` |
| `hidden` | bool | `true` |

未知の `k` は**無視してログに出す**（FR-4.3）。他の修飾子の適用は続ける。

### 4.1 `font` の値

`largeTitle` `title` `title2` `title3` `headline` `subheadline`
`body` `callout` `footnote` `caption` `caption2`

> design §4.1 は `title` / `body` / `caption` / `headline` の 4 つを例示していたが、
> SwiftUI の標準テキストスタイル一式に揃えた。switch の分岐が増えるだけで実装コストは
> ほぼ変わらず、恣意的な制限を避けられる。

### 4.2 色の値

**名前つき**: `primary` `secondary` `red` `orange` `yellow` `green` `mint` `teal`
`cyan` `blue` `indigo` `purple` `pink` `brown` `gray` `black` `white` `clear`

**16 進**: `#RRGGBB`（大文字小文字は問わない）

解釈できない値は無視してログに出す。

### 4.3 `frame` の `maxWidth` / `maxHeight`

数値、または文字列 `"infinity"` を取る。`"infinity"` は SwiftUI の `.infinity` に対応する。

---

## 5. イベント

`payload` は**閉じたタグ付き共用体**である（FR-3.5）。任意の JSON は許さない。
Go 側がハンドラを型で分岐するため、こちらはタグが必要になる。

```jsonc
{ "t": "none" }
{ "t": "string", "v": "..." }
{ "t": "bool",   "v": true }
{ "t": "int",    "v": 3 }
{ "t": "double", "v": 1.5 }
{ "t": "index",  "v": 2 }
```

### 5.1 `kind` とノード型の対応（v0.1）

| ノード型 | `kind` | `payload` |
|---|---|---|
| `Button` | `tap` | `none` |
| `Toggle` | `change` | `bool` |
| `TextField` | `commit` | `string` |

未知の `kind` や未知の `nodeID` へのイベントは**無視してログに出す**。panic しない。

---

## 6. ID 規約

- 既定は**構造パス**。ルートが `0`、その i 番目の子が `0/i`、以下同様
- `.ID("x")` で明示的に上書きできる。**その場合、子孫の接頭辞も `x` になる**
- **明示 ID に `/` を含めてはならない。** Go 側は採番時に `invalid_id` エラーとする
  （`.ID("x")` の子 `x/0` と `.ID("x/0")` が衝突しうるため）
- 同一ツリー内での重複は Go 側が `duplicate_id` エラーとする。**ホストは一意性を前提にしてよい**

### 6.1 `ForEach` はワイヤに現れる（設計への追加）

`ForEach` は Go 側の構文だが、**ワイヤ上にもノードとして出す**。行を親へ平坦化しない。

理由は**兄弟の構造インデックスがずれるため**である。平坦化すると次のようになる。

```go
VStack(
    ForEach(items, byID, row),   // 行数ぶんの子に展開される
    Text("footer"),              // ← items の件数で位置が変わる
)
```

`items` が 3 件なら footer は `0/3`、4 件なら `0/4` になり、**項目を足すたびに footer の ID が
変わって SwiftUI がビューを作り直す**。`ForEach` をノードとして残せば footer は常に `0/1` である。

インタープリタ側の追加コストは switch の 1 分岐だけで済む。

```swift
case "ForEach":
    return AnyView(ForEach(node.children, id: \.id) { SuigoNodeView(node: $0).id($0.id) })
```

---

## 7. 上限

| 項目 | 値 | 検出側 |
|---|---|---|
| フレーム長 | 16 MB | dev トランスポートの受信側 |
| ツリーの深さ | 256 | Swift の復号時（防御）/ Go の採番時（早期検出） |
| ノード数 | 制限なし。**300 超で `node_count_warning`** | Go の描画時 |

---

## 8. フィクスチャ

`testdata/wire/` に置く。Go 側と Swift 側の**両方が同じファイルを参照する**。

| ファイル | 用途 |
|---|---|
| `golden/render_counter.json` | カウンター相当（5 ノード） |
| `golden/render_todo3.json` | TODO 3 件（`ForEach` と明示 ID を含む） |
| `golden/render_all_primitives.json` | 14 種すべて |
| `golden/render_all_modifiers.json` | 修飾子すべて（順序込み） |
| `golden/render_deep_100.json` | 深さ 100 |
| `golden/events.json` | 6 種のペイロードすべて |
| `golden/env.json` | `env` フレーム |
| `golden/errors.json` | `error` フレーム 6 種 |
| `invalid/unknown_node.json` | 未知ノード型（既知部分は描画されること） |
| `invalid/unknown_modifier.json` | 未知修飾子 |
| `invalid/wrong_prop_type.json` | `props` の型違い |
| `invalid/missing_prop.json` | 必須 `props` の欠落 |
| `invalid/unknown_frame.json` | 未知のフレーム型 |

**golden の比較は「復号した構造の一致」で行う。バイト列一致に依存しない**（FR-3.7）。
`Encoder` を差し替えてもテストが通るようにするためである。
