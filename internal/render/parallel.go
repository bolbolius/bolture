package render

import "sync"

// Parallel work is split into fixed row ranges, one per worker. Error
// diffusion is sequential and stays out of the pool; sampling and drawing are
// row-independent and safe to split.
const (
	stepSample = iota + 1
	stepFill
)

type parallelPool struct {
	r     *Renderer
	chans []chan struct{}
	done  sync.WaitGroup

	// Set by the caller before a phase and read by workers after they receive
	// their signal. The channel send and the WaitGroup provide the ordering.
	step  int
	rows  int
	chunk int
}

func newParallelPool(r *Renderer, n int) *parallelPool {
	p := &parallelPool{r: r, chans: make([]chan struct{}, n)}
	for i := range p.chans {
		p.chans[i] = make(chan struct{}, 1)
		go p.worker(i)
	}
	return p
}

func (p *parallelPool) worker(id int) {
	ch := p.chans[id]
	for range ch {
		lo := id * p.chunk
		hi := lo + p.chunk
		if hi > p.rows {
			hi = p.rows
		}
		if lo < hi {
			p.r.runRows(p.step, lo, hi)
		}
		p.done.Done()
	}
}

func (p *parallelPool) run(step, rows int) {
	p.step = step
	p.rows = rows
	p.chunk = (rows + len(p.chans) - 1) / len(p.chans)
	p.done.Add(len(p.chans))
	for _, ch := range p.chans {
		ch <- struct{}{}
	}
	p.done.Wait()
}

func (r *Renderer) runRows(step, lo, hi int) {
	switch step {
	case stepSample:
		r.sampleRows(lo, hi)
	case stepFill:
		r.fillRows(lo, hi)
	}
}

// Close stops the workers. Call it when no Render is in flight. It is safe to
// call more than once and on a renderer that never enabled multi-core.
func (r *Renderer) Close() {
	if r.pool == nil {
		return
	}
	for _, ch := range r.pool.chans {
		close(ch)
	}
	r.pool = nil
}
