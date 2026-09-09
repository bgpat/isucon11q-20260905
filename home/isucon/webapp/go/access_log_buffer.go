package main

import (
	"io"
	"sync"
	"time"
)

// deferredLogWriter は負荷走行中のアクセスログをメモリに積み、
// flush まで dst へ出さない。middleware.Logger の Output に渡す。
type deferredLogWriter struct {
	mu        sync.Mutex
	dst       io.Writer
	buf       []byte
	buffering bool
	epoch     uint64
	timer     *time.Timer
}

const accessLogBufInit = 16 << 20

func newDeferredLogWriter(dst io.Writer) *deferredLogWriter {
	return &deferredLogWriter{dst: dst}
}

func (w *deferredLogWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	if w.buffering {
		w.buf = append(w.buf, p...)
		n := len(p)
		w.mu.Unlock()
		return n, nil
	}
	dst := w.dst
	w.mu.Unlock()
	return dst.Write(p)
}

func (w *deferredLogWriter) startBuffering(d time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.flushLocked()
	if cap(w.buf) < accessLogBufInit {
		w.buf = make([]byte, 0, accessLogBufInit)
	}
	w.buffering = true
	w.epoch++
	epoch := w.epoch
	if w.timer != nil {
		w.timer.Stop()
	}
	w.timer = time.AfterFunc(d, func() {
		w.flushEpoch(epoch)
	})
}

func (w *deferredLogWriter) Flush() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.epoch++
	if w.timer != nil {
		w.timer.Stop()
		w.timer = nil
	}
	w.flushLocked()
	w.buffering = false
}

func (w *deferredLogWriter) flushEpoch(epoch uint64) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.epoch != epoch {
		return
	}
	w.flushLocked()
	w.buffering = false
	w.timer = nil
}

func (w *deferredLogWriter) flushLocked() {
	if len(w.buf) == 0 {
		return
	}
	_, _ = w.dst.Write(w.buf)
	w.buf = w.buf[:0]
}
