//go:build suigodev

// Package devserver は dev トランスポートの Go 子プロセス側を実装する。
// stdin からフレームを読み、stdout へフレームを書く。TCP や監視は cmd/suigo（親）が担う。
//
// このパッケージ全体が suigodev ビルドタグで隔離されており、本番の c-archive には含まれない（AR-2）。
package devserver

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/fummicc1/suigo/internal/runtime"
	"github.com/fummicc1/suigo/internal/wire"
	"github.com/fummicc1/suigo/suigo"
)

// import されるだけで suigo.Run が dev サーバに接続されるようにする。
// 利用者の main は `_ "github.com/fummicc1/suigo/devserver"` と副作用 import すればよい。
func init() { suigo.SetDevServe(Serve) }

// Serve は登録済みのアプリを stdin/stdout に接続し、ctx が終わるか stdin が閉じるまで戻らない。
func Serve(ctx context.Context) error {
	app, err := suigo.Registered()
	if err != nil {
		return err
	}
	return serve(ctx, app, os.Stdin, os.Stdout)
}

func serve(ctx context.Context, app suigo.App, in io.Reader, out io.Writer) error {
	enc := wire.JSON{}
	loop := runtime.New(app, enc)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wmu sync.Mutex
	write := func(b []byte) error {
		wmu.Lock()
		defer wmu.Unlock()
		return wire.WriteFrame(out, b)
	}

	// mount が来るまで描画を始めない。restore を先に適用するため。
	mounted := make(chan []byte, 1)

	go func() {
		defer cancel()
		for {
			b, err := wire.ReadFrame(in)
			if err != nil {
				return
			}
			typ, err := wire.PeekType(enc, b)
			if err != nil {
				continue
			}
			switch typ {
			case "mount":
				var f wire.MountFrame
				if err := enc.Decode(b, &f); err == nil {
					select {
					case mounted <- f.Restore:
					default:
					}
				}
			case "event":
				var f wire.EventFrame
				if err := enc.Decode(b, &f); err == nil {
					loop.Dispatch(f.NodeID, f.Kind, wire.ToPayload(f.Payload))
				}
			case "env":
				// v0.1 では env を受け取るだけで再描画契機にはしない（design §11-2）。
			case "snapshot":
				snap, err := loop.Snapshot(ctx)
				var reply wire.SnapshotResultFrame
				reply.Type = "snapshot_result"
				if err != nil {
					reply.Error = err.Error()
				} else {
					reply.Model = rawJSON(snap)
				}
				if rb, err := enc.Encode(reply); err == nil {
					_ = write(rb)
				}
			}
		}
	}()

	var restore []byte
	select {
	case restore = <-mounted:
	case <-ctx.Done():
		return ctx.Err()
	}

	// Restore は Run より前に済ませる。Run は開始直後に初回描画を出すため、
	// 後から復元すると「ゼロ値の描画 → 復元後の描画」の順になり、その隙間に
	// 次の snapshot 要求が来ると復元前の値を返してしまう（実測で状態喪失が再現した）。
	if len(restore) > 0 && !bytes.Equal(restore, []byte("null")) {
		// 失敗してもゼロ値で続行する（FR-6.4）。warning は loop が出す。
		_ = app.Restore(restore)
	}

	go loop.Run(ctx)

	for {
		b, ok := loop.NextFrame(ctx)
		if !ok {
			if errors.Is(ctx.Err(), context.Canceled) {
				return nil
			}
			return ctx.Err()
		}
		if err := write(b); err != nil {
			return fmt.Errorf("write frame: %w", err)
		}
	}
}

// rawJSON は既に JSON であるバイト列を、二重に符号化せずそのまま埋め込むための型。
type rawJSON []byte

func (r rawJSON) MarshalJSON() ([]byte, error) {
	if len(r) == 0 {
		return []byte("null"), nil
	}
	return r, nil
}
