package suigo

import (
	"encoding/json"
	"errors"
	"sync"
)

// App はジェネリクスを消去したアプリの姿。トランスポート層はこれだけを見る。
//
// -buildmode=c-archive では main.main が実行されず、起動は suigo_mount() から始まる。
// そのため利用者は init() で Register を呼んでアプリを登録し、dev では main() から
// Run を呼ぶ。同じ登録が両経路から到達できる。
type App interface {
	Wake() <-chan struct{}
	Apply() int
	View() Node
	Snapshot() ([]byte, error)
	Restore([]byte) error
}

var (
	registryMu sync.Mutex
	registered App
)

// ErrAlreadyRegistered は Register を 2 回呼んだときに返る。
var ErrAlreadyRegistered = errors.New("suigo: app already registered")

// ErrNotRegistered は登録前に Registered を呼んだときに返る。
var ErrNotRegistered = errors.New("suigo: no app registered; call suigo.Register in init()")

// Register はアプリを登録する。init() から 1 回だけ呼ぶ。
//
// モデル M は encoding/json で往復できる型でなければならない。ホットリロード時の
// 状態引き継ぎに使うためである。エクスポートされていないフィールドは引き継がれない。
func Register[M any](initial M, view func(M, *Store[M]) Node) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if registered != nil {
		panic(ErrAlreadyRegistered)
	}
	registered = &app[M]{store: NewStore(initial), view: view}
}

// Registered は登録済みのアプリを返す。
func Registered() (App, error) {
	registryMu.Lock()
	defer registryMu.Unlock()
	if registered == nil {
		return nil, ErrNotRegistered
	}
	return registered, nil
}

// resetRegistry はテスト用。
func resetRegistry() {
	registryMu.Lock()
	registered = nil
	registryMu.Unlock()
}

type app[M any] struct {
	store *Store[M]
	view  func(M, *Store[M]) Node
}

func (a *app[M]) Wake() <-chan struct{} { return a.store.Wake() }
func (a *app[M]) Apply() int            { return a.store.Apply() }
func (a *app[M]) View() Node            { return a.view(a.store.Model(), a.store) }

func (a *app[M]) Snapshot() ([]byte, error) { return json.Marshal(a.store.Model()) }

func (a *app[M]) Restore(b []byte) error {
	var m M
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	a.store.Replace(m)
	return nil
}
