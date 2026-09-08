package main

import (
	"net"
	"os"
	"path/filepath"
	"testing"
)

func TestListenUnixSocketMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "isucondition.sock")
	ln, err := listenUnixSocket(path)
	if err != nil {
		t.Fatalf("listenUnixSocket: %v", err)
	}
	defer ln.Close()

	st, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if st.Mode()&os.ModeSocket == 0 {
		t.Fatalf("not a socket: %v", st.Mode())
	}
	if got, want := st.Mode().Perm(), os.FileMode(0660); got != want {
		t.Fatalf("perm=%o want %o", got, want)
	}

	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	conn.Close()
}
