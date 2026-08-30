package wire

import "encoding/json"

// Encoder はフレームをバイト列に符号化する。形式を差し替えられるよう
// インタフェースにしている（FR-3.7）。golden テストは復号した構造で比較し、
// バイト列には依存しない。
type Encoder interface {
	Encode(frame any) ([]byte, error)
	Decode(data []byte, into any) error
}

// JSON は既定のエンコーダ。
type JSON struct{}

func (JSON) Encode(frame any) ([]byte, error) { return json.Marshal(frame) }
func (JSON) Decode(data []byte, into any) error { return json.Unmarshal(data, into) }

// PeekType はフレームの "type" だけを取り出す。
// 全体を復号する前にどの構造体へ入れるかを決めるために使う。
func PeekType(enc Encoder, data []byte) (string, error) {
	var head struct {
		Type string `json:"type"`
	}
	if err := enc.Decode(data, &head); err != nil {
		return "", err
	}
	return head.Type, nil
}
