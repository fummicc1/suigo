//go:build cgo

// Package embedded は本番経路の C ABI を公開する。Go を c-archive としてアプリにリンクし、
// フレームをバイトバッファだけで双方向に流す（design §2.2）。
//
// 利用者は小さな main パッケージからこのパッケージと自分の ui パッケージを副作用 import する。
// -buildmode=c-archive では main.main が実行されず、起動は suigo_mount() から始まる。
//
// //export 関数は main 以外のパッケージにあってもシンボルとしてアーカイブに載る（Task 1.2 で確認）。
// ただしヘッダは生成されないため、Sources/CSuigo/suigo.h を手書きで同梱している。
package embedded

/*
#include <stdlib.h>
*/
import "C"

import (
	"context"
	"sync"
	"unsafe"

	"github.com/fummicc1/suigo/internal/runtime"
	"github.com/fummicc1/suigo/internal/wire"
	"github.com/fummicc1/suigo/suigo"
)

type instance struct {
	loop   *runtime.Loop
	cancel context.CancelFunc
	ctx    context.Context
}

var (
	mu     sync.Mutex
	insts  = map[int]*instance{}
	nextID = 1
)

func lookup(h C.int) *instance {
	mu.Lock()
	defer mu.Unlock()
	return insts[int(h)]
}

// suigo_mount は登録済みアプリのレンダーループを起動し、ハンドルを返す。失敗時は負値。
//
//export suigo_mount
func suigo_mount() C.int {
	app, err := suigo.Registered()
	if err != nil {
		return -1
	}
	ctx, cancel := context.WithCancel(context.Background())
	loop := runtime.New(app, wire.JSON{})
	in := &instance{loop: loop, cancel: cancel, ctx: ctx}

	mu.Lock()
	h := nextID
	nextID++
	insts[h] = in
	mu.Unlock()

	go loop.Run(ctx)
	return C.int(h)
}

// suigo_unmount はループを止める。ブロック中の suigo_next_frame は NULL を返して戻る。
//
//export suigo_unmount
func suigo_unmount(h C.int) {
	mu.Lock()
	in := insts[int(h)]
	delete(insts, int(h))
	mu.Unlock()
	if in != nil {
		in.cancel()
	}
}

// suigo_send はホスト → Go のフレームを渡す。呼び出し中にコピーするため、
// 呼び出し元は戻った直後にバッファを解放してよい。成功で 0。
//
//export suigo_send
func suigo_send(h C.int, buf unsafe.Pointer, n C.int) C.int {
	in := lookup(h)
	if in == nil {
		return -1
	}
	if buf == nil || n <= 0 {
		return -2
	}
	b := C.GoBytes(buf, n)
	enc := wire.JSON{}
	typ, err := wire.PeekType(enc, b)
	if err != nil {
		return -3
	}
	switch typ {
	case "event":
		var f wire.EventFrame
		if err := enc.Decode(b, &f); err != nil {
			return -3
		}
		in.loop.Dispatch(f.NodeID, f.Kind, wire.ToPayload(f.Payload))
	case "mount", "env":
		// mount は suigo_mount 自体が担う。env は v0.1 では再描画契機にしない（design §11-2）。
	default:
		// 未知のフレーム型は無視する（docs/wire-format.md §2.3）。
	}
	return 0
}

// suigo_next_frame は次のフレームが出るまでブロックし、malloc したバッファを返す。
// 呼び出し元は Data へコピーしたあと suigo_free で解放する。unmount 後は NULL を返す。
//
// メインスレッドから呼んではならない。専用スレッドで回すこと（design §2.2 のスレッド規約）。
//
//export suigo_next_frame
func suigo_next_frame(h C.int, outLen *C.int) unsafe.Pointer {
	in := lookup(h)
	if in == nil {
		*outLen = 0
		return nil
	}
	b, ok := in.loop.NextFrame(in.ctx)
	if !ok {
		*outLen = 0
		return nil
	}
	*outLen = C.int(len(b))
	return C.CBytes(b)
}

// suigo_free は suigo_next_frame が返したバッファを解放する。
//
//export suigo_free
func suigo_free(p unsafe.Pointer) {
	if p != nil {
		C.free(p)
	}
}
