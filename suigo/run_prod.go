//go:build !suigodev

package suigo

import (
	"context"
	"errors"
)

// ErrDevNotBuilt は suigodev タグ無しで Run を呼んだときに返る。
// 本番の c-archive では Run は使われない（suigo_mount が入口になる）。
var ErrDevNotBuilt = errors.New("suigo: Run requires -tags suigodev; production builds start via suigo_mount")

func runDev(context.Context) error { return ErrDevNotBuilt }
