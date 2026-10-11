package uniswapquote

func (w *Wrapper) Cached() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.cache)
}
