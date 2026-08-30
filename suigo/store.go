package suigo

// Store はモデルを保持し、変更の予約を受け付ける。
//
// モデルに触れてよいのはレンダーループの goroutine だけである。Update は変更関数を
// キューへ積むだけで、その場では適用しない。レンダーループが描画の直前に溜まった
// 変更をまとめて適用する。読み書きする goroutine が 1 本しかないため、ミューテックスが
// 不要になり、データ競合の余地そのものが無くなる。
//
// 帰結として、Update で積んだ変更はその呼び出しの直後にはまだ反映されていない。
// ハンドラ内で「積んだ直後に読み直す」ことはできない。読みたい値は View が受け取る
// モデルから得る。
type Store[M any] struct {
	model     M
	mutations chan func(*M)
	wake      chan struct{}
}

// NewStore は初期モデルを持つ Store を作る。
func NewStore[M any](initial M) *Store[M] {
	return &Store[M]{
		model:     initial,
		mutations: make(chan func(*M), 1024),
		// バッファ 1。積まれた変更が何件あっても「起きろ」の合図は 1 つで足りる。
		wake: make(chan struct{}, 1),
	}
}

// Update は変更を予約する。任意の goroutine から呼べ、すぐに戻る。
// キューが満杯なら呼び出し側をブロックする（変更を捨てるよりはよい）。
func (s *Store[M]) Update(mutate func(*M)) {
	if mutate == nil {
		return
	}
	s.mutations <- mutate
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

// Wake はレンダーループが待つチャネル。変更が積まれると 1 つ送られる。
func (s *Store[M]) Wake() <-chan struct{} { return s.wake }

// Apply は溜まった変更を積まれた順にすべて適用し、件数を返す。
// レンダーループの goroutine からのみ呼ぶこと。
func (s *Store[M]) Apply() int {
	n := 0
	for {
		select {
		case m := <-s.mutations:
			m(&s.model)
			n++
		default:
			return n
		}
	}
}

// Model は現在のモデルを返す。レンダーループの goroutine からのみ呼ぶこと。
func (s *Store[M]) Model() M { return s.model }

// Replace はモデルを丸ごと置き換える。ホットリロード時の復元に使う。
// レンダーループの goroutine からのみ呼ぶこと。
func (s *Store[M]) Replace(m M) { s.model = m }
