package suigo

import (
	"sync"
	"testing"
)

type counter struct{ Count int }

type todoModel struct{ Items []string }

// TG-801: Update → Apply で状態が変わる
func TestUpdateThenApply(t *testing.T) {
	s := NewStore(counter{})
	s.Update(func(m *counter) { m.Count++ })
	if s.Model().Count != 0 {
		t.Error("Update must not apply immediately")
	}
	if n := s.Apply(); n != 1 {
		t.Errorf("applied %d, want 1", n)
	}
	if s.Model().Count != 1 {
		t.Errorf("Count = %d, want 1", s.Model().Count)
	}
}

// TG-802: 100 件積んでも wake は 1 回、Apply で全件適用
func TestManyUpdatesCoalesceIntoOneWake(t *testing.T) {
	s := NewStore(counter{})
	for i := 0; i < 100; i++ {
		s.Update(func(m *counter) { m.Count++ })
	}
	wakes := 0
	for {
		select {
		case <-s.Wake():
			wakes++
			continue
		default:
		}
		break
	}
	if wakes != 1 {
		t.Errorf("wake signals = %d, want exactly 1 (coalesced)", wakes)
	}
	if n := s.Apply(); n != 100 {
		t.Errorf("applied %d, want 100", n)
	}
	if s.Model().Count != 100 {
		t.Errorf("Count = %d, want 100", s.Model().Count)
	}
}

// TG-803: View（＝Apply 後の Model()）は積まれた変更すべてを反映している
func TestModelReflectsAllPendingAfterApply(t *testing.T) {
	s := NewStore(todoModel{})
	s.Update(func(m *todoModel) { m.Items = append(m.Items, "a") })
	s.Update(func(m *todoModel) { m.Items = append(m.Items, "b") })
	s.Update(func(m *todoModel) { m.Items = append(m.Items, "c") })
	s.Apply()
	if got := s.Model().Items; len(got) != 3 || got[0] != "a" || got[2] != "c" {
		t.Errorf("Items = %v", got)
	}
}

// TG-804: スライスを含むモデルへ複数 goroutine から同時に Update しても、
// -race で競合が出ず、1 件も失われない。モデルに触る goroutine が 1 本だけだから成り立つ。
func TestConcurrentUpdatesAreRaceFreeAndLossless(t *testing.T) {
	s := NewStore(todoModel{})
	const workers, each = 8, 50

	var wg sync.WaitGroup
	done := make(chan struct{})

	// レンダーループ役: Wake を受けるたびに Apply し、Model を読む（＝View の代わり）
	applied := 0
	go func() {
		for {
			select {
			case <-s.Wake():
				applied += s.Apply()
				_ = len(s.Model().Items)
			case <-done:
				applied += s.Apply()
				close(done)
				return
			}
		}
	}()

	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			for i := 0; i < each; i++ {
				s.Update(func(m *todoModel) { m.Items = append(m.Items, "x") })
			}
		}(w)
	}
	wg.Wait()
	done <- struct{}{}
	<-done

	if applied != workers*each {
		t.Errorf("applied %d, want %d (lost updates)", applied, workers*each)
	}
	if got := len(s.Model().Items); got != workers*each {
		t.Errorf("Items = %d, want %d", got, workers*each)
	}
}

// TG-806: 積んだ変更はその場では見えない（設計上の制約を固定する）
func TestUpdateIsNotVisibleUntilApply(t *testing.T) {
	s := NewStore(counter{})
	s.Update(func(m *counter) { m.Count = 42 })
	if s.Model().Count == 42 {
		t.Fatal("Update must be deferred until Apply; this is a documented constraint")
	}
	s.Apply()
	if s.Model().Count != 42 {
		t.Error("value must be visible after Apply")
	}
}

// TG-807: 適用順は積んだ順
func TestApplyOrderMatchesUpdateOrder(t *testing.T) {
	s := NewStore(todoModel{})
	for _, v := range []string{"1", "2", "3"} {
		v := v
		s.Update(func(m *todoModel) { m.Items = append(m.Items, v) })
	}
	s.Apply()
	if got := s.Model().Items; got[0] != "1" || got[1] != "2" || got[2] != "3" {
		t.Errorf("order = %v", got)
	}
}

func TestNilUpdateIgnored(t *testing.T) {
	s := NewStore(counter{})
	s.Update(nil)
	if n := s.Apply(); n != 0 {
		t.Errorf("nil update applied %d times", n)
	}
}

func TestReplace(t *testing.T) {
	s := NewStore(counter{Count: 1})
	s.Replace(counter{Count: 99})
	if s.Model().Count != 99 {
		t.Error("Replace failed")
	}
}
