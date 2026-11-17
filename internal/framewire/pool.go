package framewire

import "sync"

// Pool keeps the bodies of frames once nobody reads them any more, and
// hands them to the next frames Read takes in, so a play makes no new
// memory for its frames. A frame of the video preview is 5.8 MB at 1600
// by 900, and a play reads 30 a second: each in a buffer of its own was
// about 165 MB a second for Go to clean up after. It is the Go side's
// half of what frontend/src/lib/frames/pool.ts does on the page.
//
// A body is only ever given back by whoever takes the frame last, once
// it has written it on, so no frame's pixels change under anyone. A body
// that is never given back is cleaned up as any other memory is. A nil
// Pool keeps nothing, and Read then makes every body new.
type Pool struct {
	mu    sync.Mutex
	free  map[int][][]byte
	kept  int
	limit int
}

// NewPool is a pool that keeps up to limit bytes of bodies nobody reads.
func NewPool(limit int) *Pool {
	return &Pool{free: map[int][][]byte{}, limit: limit}
}

// take is a body of size bytes kept from a frame before, or nil when
// there is none.
func (p *Pool) take(size int) []byte {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	list := p.free[size]
	if len(list) == 0 {
		return nil
	}
	b := list[len(list)-1]
	p.free[size] = list[:len(list)-1]
	p.kept -= cap(b)
	return b[:size]
}

// Give hands back the body of a frame nobody reads any more. Bodies of
// another size than this one go first when the pool is full, since
// frames change size only when the app does.
func (p *Pool) Give(b []byte) {
	if p == nil || cap(b) == 0 {
		return
	}
	b = b[:cap(b)]
	p.mu.Lock()
	defer p.mu.Unlock()
	for size, list := range p.free {
		if size == len(b) {
			continue
		}
		for len(list) > 0 && p.kept+len(b) > p.limit {
			p.kept -= cap(list[len(list)-1])
			list = list[:len(list)-1]
		}
		p.free[size] = list
	}
	if p.kept+len(b) > p.limit {
		return
	}
	p.free[len(b)] = append(p.free[len(b)], b)
	p.kept += len(b)
}

// Clear lets go of every body kept, once no frame is coming.
func (p *Pool) Clear() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	clear(p.free)
	p.kept = 0
}
