package wire

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// MaxFrameLen はフレーム本体の上限。docs/wire-format.md §7。
// snapshot_result が利用者のモデルを丸ごと運ぶため、描画ツリーの規模より大きく取る。
const MaxFrameLen = 16 * 1024 * 1024

var (
	ErrFrameTooLarge = errors.New("frame exceeds maximum length")
	ErrEmptyFrame    = errors.New("frame length is zero")
)

// WriteFrame は 4 バイトのリトルエンディアン長さ接頭辞を付けて書く。
func WriteFrame(w io.Writer, body []byte) error {
	if len(body) == 0 {
		return ErrEmptyFrame
	}
	if len(body) > MaxFrameLen {
		return fmt.Errorf("%w: %d bytes", ErrFrameTooLarge, len(body))
	}
	var hdr [4]byte
	binary.LittleEndian.PutUint32(hdr[:], uint32(len(body)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := w.Write(body)
	return err
}

// ReadFrame は長さ接頭辞つきのフレームを 1 つ読む。
// 上限超過・長さ 0 はエラーとし、呼び出し側は接続を切るべきである。
func ReadFrame(r io.Reader) ([]byte, error) {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return nil, err
	}
	n := binary.LittleEndian.Uint32(hdr[:])
	if n == 0 {
		return nil, ErrEmptyFrame
	}
	if n > MaxFrameLen {
		return nil, fmt.Errorf("%w: %d bytes", ErrFrameTooLarge, n)
	}
	body := make([]byte, n)
	if _, err := io.ReadFull(r, body); err != nil {
		return nil, err
	}
	return body, nil
}
