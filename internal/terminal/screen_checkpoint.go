package terminal

import "sshc/third_party/vt"

// 流し直しのバッファ（Ring）は、いちばん新しい出力しか持たない。新しく始めるブラウザのターミナルや、
// バッファが届く範囲より長く離れていたターミナルは、その分しか流し直せない。それより前に画面を描いた
// プログラム、たとえば変わったところだけを描き直す全画面のプログラム（Claude Code）は、描いたものが無い
// まま出てしまう。そこで、tmux がペインに対してするように、エンジンは自前のターミナルエミュレーターで出力を
// 追う。読むのはバッファが手放したものだけなので、その画面は、残っているいちばん古いバイトが書かれたときの
// 画面である。そういうブラウザには、まずその画面を送り、続けて残っている出力をそのまま送る
// （Session.readOutputLocked）。こうすると、いちばん新しい出力は、プログラムが書いたとおりのままになる。
type screenCheckpoint struct {
	// emulator は、position より前の出力を読み終えている。バッファが初めて出力を手放したときに作る。それまでは、
	// 最初のバイトから流し直せば、すべてを再現できる。
	emulator screenEmulator
	position uint64
	// newEmulator は、エミュレーターを作る。nil なら vt.SnapshotEmulator を作る。
	newEmulator func(Size) screenEmulator
	// sizes は、ターミナルの大きさである。それぞれ、決まったときの出力の位置を持ち、古い順に並ぶ。最初のものは、
	// エミュレーターが今持っている（または作るときに使う）大きさである。エミュレーターは、次の大きさの位置まで
	// 読んだら、その大きさに変える。
	sizes []sizeChange
}

type sizeChange struct {
	at   uint64
	size Size
}

// screenEmulator は、出力を追う。vt.SnapshotEmulator か、試験が代わりに入れるものである。
type screenEmulator interface {
	Write(output []byte) (int, error)
	Resize(cols, rows int)
	// AtGround は、ここまで読んだ出力が、エスケープシーケンスや UTF-8 の文字の途中で終わっていないことを返す。
	AtGround() bool
	// Snapshot は、エミュレーターが出しているものを出すシーケンスを返す。
	Snapshot() string
}

func newScreenCheckpoint(size Size) screenCheckpoint {
	return screenCheckpoint{sizes: []sizeChange{{at: 0, size: size}}}
}

// sizesFrom は、位置 start の出力が書かれたときの大きさと、そのあとの変更を1つずつ返す。ブラウザが、出力を
// 書かれたときの幅で流し直すのに要るものである（Replay.Sizes）。大きさは、バッファが持ついちばん古い出力まで
// さかのぼるので、start が最初のものより古いことは無い。
func (c *screenCheckpoint) sizesFrom(start uint64) []SizeAt {
	var sizes []SizeAt
	for _, change := range c.sizes {
		if change.at <= start {
			sizes = []SizeAt{{At: start, Size: change.size}}
			continue
		}
		sizes = append(sizes, SizeAt{At: change.at, Size: change.size})
	}
	return sizes
}

// resized は、位置 at 以降の出力が size に向けて書かれることを記録する。
func (c *screenCheckpoint) resized(at uint64, size Size) {
	if last := len(c.sizes) - 1; last > 0 && c.sizes[last].at == at {
		c.sizes[last].size = size
	} else {
		c.sizes = append(c.sizes, sizeChange{at: at, size: size})
	}
	c.takeSizes()
}

// keepUp は、chunk を書くと buffer から押し出される出力を読む。書く前に呼ぶ。
func (c *screenCheckpoint) keepUp(buffer *Ring, chunk []byte) {
	written := buffer.Written()
	end := written + uint64(len(chunk))
	if end <= uint64(buffer.Capacity()) {
		return
	}
	pushedOut := end - uint64(buffer.Capacity())
	if pushedOut <= c.position {
		return
	}
	if c.emulator == nil {
		c.emulator = c.makeEmulator()
		c.takeSizes()
	}
	if c.position < written {
		retained, _ := buffer.ReadAvailableFrom(c.position, int(min(pushedOut, written)-c.position))
		c.read(retained.Data)
	}
	if pushedOut > written {
		c.read(chunk[:pushedOut-written])
	}
}

// settleStep は、settle が、残っている出力を1度に読む量である。たいていのシーケンスはその中で終わる。
// インラインの画像は何度かかかる。
const settleStep = 256

// settle は、エミュレーターがエスケープシーケンスや UTF-8 の文字の途中にいるあいだ、バッファから先を読む。
// 流し直しを、チェックポイントの位置から始められるようにするためである。そこからなら、出力は元と同じように
// 解釈される。書いたあとに呼ぶ。
func (c *screenCheckpoint) settle(buffer *Ring) {
	for c.emulator != nil && !c.emulator.AtGround() && c.position < buffer.Written() {
		retained, _ := buffer.ReadAvailableFrom(c.position, settleStep)
		for index := range retained.Data {
			c.read(retained.Data[index : index+1])
			if c.emulator.AtGround() {
				return
			}
		}
	}
}

// snapshot は、チェックポイントの画面を出すシーケンスと、それが表す出力の位置を返す。チェックポイントが
// あるのは、バッファが出力を手放したあとである。
func (c *screenCheckpoint) snapshot() (string, uint64) {
	return c.emulator.Snapshot(), c.position
}

func (c *screenCheckpoint) makeEmulator() screenEmulator {
	size := DefaultSize()
	if len(c.sizes) > 0 {
		size = c.sizes[0].size
	}
	if c.newEmulator != nil {
		return c.newEmulator(size)
	}
	return vt.NewSnapshotEmulator(int(size.Cols), int(size.Rows))
}

// read は、c.position 以降の出力をエミュレーターに渡す。大きさは、それぞれ決まった位置で当てる。
func (c *screenCheckpoint) read(output []byte) {
	for len(output) > 0 {
		piece := output
		if len(c.sizes) > 1 && c.sizes[1].at < c.position+uint64(len(piece)) {
			piece = output[:c.sizes[1].at-c.position]
		}
		_, _ = c.emulator.Write(piece)
		c.position += uint64(len(piece))
		output = output[len(piece):]
		c.takeSizes()
	}
}

func (c *screenCheckpoint) takeSizes() {
	if c.emulator == nil {
		return
	}
	resized := false
	for len(c.sizes) > 1 && c.sizes[1].at <= c.position {
		c.sizes = c.sizes[1:]
		resized = true
	}
	if resized {
		c.emulator.Resize(int(c.sizes[0].size.Cols), int(c.sizes[0].size.Rows))
	}
}
