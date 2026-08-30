package suigo

import (
	"errors"
	"testing"
)

type appModel struct {
	Count  int
	hidden int // エクスポートされていないので Snapshot に含まれない
}

func TestRegisterAndRegistered(t *testing.T) {
	resetRegistry()
	defer resetRegistry()

	if _, err := Registered(); !errors.Is(err, ErrNotRegistered) {
		t.Errorf("before Register: err = %v", err)
	}
	Register(appModel{Count: 1}, func(m appModel, s *Store[appModel]) Node {
		return Text("x")
	})
	a, err := Registered()
	if err != nil {
		t.Fatal(err)
	}
	if a.View().Type != TypeText {
		t.Error("View not wired")
	}
}

func TestDoubleRegisterPanics(t *testing.T) {
	resetRegistry()
	defer resetRegistry()
	view := func(appModel, *Store[appModel]) Node { return Text("") }
	Register(appModel{}, view)
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("second Register should panic")
		}
		if err, ok := r.(error); !ok || !errors.Is(err, ErrAlreadyRegistered) {
			t.Errorf("panic value = %v", r)
		}
	}()
	Register(appModel{}, view)
}

func TestAppSnapshotRestoreRoundTrip(t *testing.T) {
	resetRegistry()
	defer resetRegistry()
	Register(appModel{Count: 5, hidden: 9}, func(appModel, *Store[appModel]) Node { return Text("") })
	a, _ := Registered()

	snap, err := a.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if string(snap) != `{"Count":5}` {
		t.Errorf("snapshot = %s (unexported fields must not appear)", snap)
	}

	if err := a.Restore([]byte(`{"Count":42}`)); err != nil {
		t.Fatal(err)
	}
	if got := a.(*app[appModel]).store.Model().Count; got != 42 {
		t.Errorf("after restore: Count = %d", got)
	}
}

func TestAppRestoreRejectsBadJSON(t *testing.T) {
	resetRegistry()
	defer resetRegistry()
	Register(appModel{}, func(appModel, *Store[appModel]) Node { return Text("") })
	a, _ := Registered()
	if err := a.Restore([]byte(`{`)); err == nil {
		t.Error("expected error")
	}
}

// フィールド名が変わった JSON でも、一致しないフィールドがゼロ値になるだけで失敗しない（FR-6.4 の前段）
func TestAppRestoreToleratesRenamedFields(t *testing.T) {
	resetRegistry()
	defer resetRegistry()
	Register(appModel{Count: 5}, func(appModel, *Store[appModel]) Node { return Text("") })
	a, _ := Registered()
	if err := a.Restore([]byte(`{"OldName":5}`)); err != nil {
		t.Fatalf("renamed field should not error: %v", err)
	}
	if got := a.(*app[appModel]).store.Model().Count; got != 0 {
		t.Errorf("Count should fall back to zero, got %d", got)
	}
}
