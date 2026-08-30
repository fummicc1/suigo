package wire

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

func TestFrameRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	for _, body := range [][]byte{[]byte("a"), []byte(`{"type":"snapshot"}`), bytes.Repeat([]byte("x"), 70000)} {
		if err := WriteFrame(&buf, body); err != nil {
			t.Fatal(err)
		}
	}
	for _, want := range []int{1, 19, 70000} {
		got, err := ReadFrame(&buf)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != want {
			t.Errorf("len = %d, want %d", len(got), want)
		}
	}
	if _, err := ReadFrame(&buf); err != io.EOF {
		t.Errorf("after all frames: err = %v, want EOF", err)
	}
}

func TestFrameHeaderIsLittleEndian(t *testing.T) {
	var buf bytes.Buffer
	_ = WriteFrame(&buf, []byte("hello"))
	hdr := buf.Bytes()[:4]
	if binary.LittleEndian.Uint32(hdr) != 5 {
		t.Errorf("header = % x, want LE 5", hdr)
	}
	if hdr[0] != 5 || hdr[3] != 0 {
		t.Errorf("not little-endian: % x", hdr)
	}
}

// TD-101 相当: 負の長さ（0xFFFFFFFF）
func TestReadRejectsHugeLength(t *testing.T) {
	r := bytes.NewReader([]byte{0xFF, 0xFF, 0xFF, 0xFF})
	_, err := ReadFrame(r)
	if !errors.Is(err, ErrFrameTooLarge) {
		t.Errorf("err = %v, want ErrFrameTooLarge", err)
	}
}

// TD-102: 上限超過（100 MB を宣言）
func TestReadRejectsOverLimit(t *testing.T) {
	var hdr [4]byte
	binary.LittleEndian.PutUint32(hdr[:], 100*1024*1024)
	_, err := ReadFrame(bytes.NewReader(hdr[:]))
	if !errors.Is(err, ErrFrameTooLarge) {
		t.Errorf("err = %v, want ErrFrameTooLarge", err)
	}
}

func TestReadAcceptsExactlyLimit(t *testing.T) {
	// 実際に 16 MB 割り当てるのではなく、ヘッダの検査だけを確かめる。
	// 本体が 0 バイトなら io.ReadFull は EOF を返す（ErrUnexpectedEOF は途中で切れた場合）。
	// どちらにせよ ErrFrameTooLarge でなければ、上限ちょうどはヘッダ検査を通過している。
	var hdr [4]byte
	binary.LittleEndian.PutUint32(hdr[:], MaxFrameLen)
	_, err := ReadFrame(bytes.NewReader(hdr[:]))
	if errors.Is(err, ErrFrameTooLarge) {
		t.Fatal("exactly MaxFrameLen must be accepted by the header check")
	}
	if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("err = %v, want an EOF variant (body missing)", err)
	}
}

// TD-103: 宣言長より本体が短い
func TestReadTruncatedBody(t *testing.T) {
	var buf bytes.Buffer
	_ = WriteFrame(&buf, []byte("hello world"))
	truncated := buf.Bytes()[:8]
	_, err := ReadFrame(bytes.NewReader(truncated))
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Errorf("err = %v, want ErrUnexpectedEOF", err)
	}
}

func TestZeroLengthRejected(t *testing.T) {
	if err := WriteFrame(io.Discard, nil); !errors.Is(err, ErrEmptyFrame) {
		t.Errorf("write: %v", err)
	}
	if _, err := ReadFrame(bytes.NewReader([]byte{0, 0, 0, 0})); !errors.Is(err, ErrEmptyFrame) {
		t.Errorf("read: %v", err)
	}
}

func TestWriteRejectsOverLimit(t *testing.T) {
	err := WriteFrame(io.Discard, make([]byte, MaxFrameLen+1))
	if !errors.Is(err, ErrFrameTooLarge) {
		t.Errorf("err = %v", err)
	}
}

func TestPeekType(t *testing.T) {
	typ, err := PeekType(JSON{}, []byte(`{"type":"event","nodeID":"0","kind":"tap","payload":{"t":"none"}}`))
	if err != nil || typ != "event" {
		t.Errorf("type=%q err=%v", typ, err)
	}
	if _, err := PeekType(JSON{}, []byte(`not json`)); err == nil {
		t.Error("expected error for invalid JSON")
	}
}
