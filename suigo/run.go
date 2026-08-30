package suigo

import "context"

// Run は登録済みのアプリを dev トランスポートで動かし、終了まで戻らない。
// main() から呼ぶ。embedded（c-archive）では main.main が実行されないため呼ばれず、
// 代わりに suigo_mount() が Registered() を通して同じアプリに到達する。
//
// 実体は devserver パッケージにあり、suigodev タグが無いビルドでは何もしない。
func Run() error {
	return runDev(context.Background())
}
