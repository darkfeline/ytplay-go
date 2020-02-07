package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"

	"golang.org/x/sys/unix"
)

// streamManager manages FIFOs created in a temporary directory and
// the streaming processes that fill the buffers feeding the FIFOs.
// Currently, this does not clean up the FIFOs, but does clean up the processes.
type streamManager struct {
	tmpdir string
	last   int
	wg     sync.WaitGroup
}

func newStreamManager(tmpdir string) *streamManager {
	return &streamManager{tmpdir: tmpdir}
}

func (m *streamManager) Wait() {
	log.Printf("Waiting for streams")
	m.wg.Wait()
}

func (m *streamManager) addStream(ctx context.Context, url string) (string, error) {
	path := m.nextFIFO()
	w, err := newBufferedFIFO(path)
	if err != nil {
		return "", err
	}
	// TODO: Clean up FIFOs somewhere.
	cmd := exec.Command("youtube-dl", "-q", "-o", "-", url)
	cmd.Stdout = w
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return "", err
	}
	log.Printf("Started youtube-dl for %s to %s", url, path)
	done := make(chan struct{})
	m.wg.Add(2)
	go func() {
		_ = cmd.Wait()
		log.Printf("youtube-dl for %s exited", path)
		_ = w.Close()
		close(done)
		m.wg.Done()
	}()
	go func() {
		select {
		case <-ctx.Done():
			log.Printf("Stopping youtube-dl for %s", path)
			_ = cmd.Process.Signal(unix.SIGTERM)
			w.Drain()
		case <-done:
		}
		m.wg.Done()
	}()
	return path, nil
}

func (m *streamManager) nextFIFO() string {
	m.last++
	return filepath.Join(m.tmpdir, fmt.Sprintf("fifo%d", m.last))
}
