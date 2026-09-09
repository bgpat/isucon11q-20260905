package main

import (
	"bytes"
	"io"
	"os"
	"sync"
	"testing"
	"time"
)

func TestAccessLogFlushDelayCoversLoad(t *testing.T) {
	if accessLogFlushDelay != benchLoadDuration+dropConditionLogIdle {
		t.Fatalf("accessLogFlushDelay=%s want %s", accessLogFlushDelay, benchLoadDuration+dropConditionLogIdle)
	}
	if benchLoadDuration < 60*time.Second {
		t.Fatalf("benchLoadDuration=%s; LOAD 60s を覆う必要がある", benchLoadDuration)
	}
	if dropConditionLogIdle < 5*time.Second {
		t.Fatalf("idle=%s; load overlap を避けるには数秒以上必要", dropConditionLogIdle)
	}
}

func TestDeferredLogWriterBuffersUntilFlush(t *testing.T) {
	dst := &bytes.Buffer{}
	w := newDeferredLogWriter(dst)
	w.startBuffering(time.Hour)

	line1 := []byte(`{"method":"GET","uri":"/api/trend","status":200}` + "\n")
	line2 := []byte(`{"method":"POST","uri":"/api/condition/x","status":202}` + "\n")
	if _, err := w.Write(line1); err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write(line2); err != nil {
		t.Fatal(err)
	}
	if dst.Len() != 0 {
		t.Fatalf("flush 前に dst へ出た: %q", dst.Bytes())
	}

	w.Flush()
	got := dst.String()
	want := string(line1) + string(line2)
	if got != want {
		t.Fatalf("flush 後 got=%q want=%q", got, want)
	}

	line3 := []byte(`{"method":"GET","uri":"/","status":200}` + "\n")
	if _, err := w.Write(line3); err != nil {
		t.Fatal(err)
	}
	if dst.String() != want+string(line3) {
		t.Fatalf("flush 後の通過 got=%q", dst.String())
	}
}

func TestDeferredLogWriterAutoFlush(t *testing.T) {
	dst := &bytes.Buffer{}
	w := newDeferredLogWriter(dst)
	w.startBuffering(30 * time.Millisecond)
	if _, err := w.Write([]byte("hello\n")); err != nil {
		t.Fatal(err)
	}
	if dst.Len() != 0 {
		t.Fatalf("flush 前に dst へ出た: %q", dst.Bytes())
	}

	deadline := time.Now().Add(time.Second)
	for dst.Len() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if dst.String() != "hello\n" {
		t.Fatalf("auto flush got=%q", dst.String())
	}
}

func TestDeferredLogWriterRestartFlushesPrevious(t *testing.T) {
	dst := &bytes.Buffer{}
	w := newDeferredLogWriter(dst)
	w.startBuffering(time.Hour)
	if _, err := w.Write([]byte("old\n")); err != nil {
		t.Fatal(err)
	}
	w.startBuffering(time.Hour)
	if dst.String() != "old\n" {
		t.Fatalf("再 initialize で前回分を出していない: %q", dst.String())
	}
	if _, err := w.Write([]byte("new\n")); err != nil {
		t.Fatal(err)
	}
	if dst.String() != "old\n" {
		t.Fatalf("新しい期間のログが先に出た: %q", dst.String())
	}
	w.Flush()
	if dst.String() != "old\nnew\n" {
		t.Fatalf("got=%q", dst.String())
	}
}

func TestDeferredLogWriterConcurrentWrites(t *testing.T) {
	dst := &bytes.Buffer{}
	w := newDeferredLogWriter(dst)
	w.startBuffering(time.Hour)

	const n = 200
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			if _, err := w.Write([]byte("x\n")); err != nil {
				t.Errorf("write: %v", err)
			}
		}()
	}
	wg.Wait()
	w.Flush()
	if got := bytes.Count(dst.Bytes(), []byte("x\n")); got != n {
		t.Fatalf("lines=%d want %d", got, n)
	}
}

func BenchmarkDeferredLogWriter(b *testing.B) {
	line := []byte(`{"time":"2026-09-10T00:00:00.000000000Z","id":"","remote_ip":"127.0.0.1","host":"localhost","method":"GET","uri":"/api/trend","user_agent":"bench","status":200,"error":"","latency":1000,"latency_human":"1µs","bytes_in":0,"bytes_out":100}` + "\n")

	b.Run("direct", func(b *testing.B) {
		f := newBenchLogFile(b)
		defer f.Close()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := f.Write(line); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("buffered", func(b *testing.B) {
		f := newBenchLogFile(b)
		defer f.Close()
		w := newDeferredLogWriter(f)
		w.startBuffering(time.Hour)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := w.Write(line); err != nil {
				b.Fatal(err)
			}
		}
		b.StopTimer()
		w.Flush()
	})
}

func newBenchLogFile(b *testing.B) *os.File {
	b.Helper()
	f, err := os.CreateTemp(b.TempDir(), "access.log")
	if err != nil {
		b.Fatal(err)
	}
	return f
}

var _ io.Writer = (*deferredLogWriter)(nil)
