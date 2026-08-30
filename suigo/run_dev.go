//go:build suigodev

package suigo

import "context"

// devServe は devserver パッケージが init() で差し込む。
// suigo → devserver の import 循環を避けるため、関数値で逆向きに結ぶ。
var devServe func(context.Context) error

func runDev(ctx context.Context) error {
	if devServe == nil {
		return ErrNotRegistered
	}
	return devServe(ctx)
}

// SetDevServe は devserver が自分自身を登録するために使う。利用者は呼ばない。
func SetDevServe(f func(context.Context) error) { devServe = f }
