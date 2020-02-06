package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

func main() {
	if err := innerMain(); err != nil {
		log.Fatal(err)
	}
}

func innerMain() error {
	tmpdir, err := ioutil.TempDir("", "ytplay")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpdir)
	log.Printf("Using tmpdir %s", tmpdir)

	files := make(chan string)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		// TODO: Handle restarting if mpv exits
		socket := filepath.Join(tmpdir, "socket")
		f, ok := <-files
		if !ok {
			return
		}
		s, err := newMPVServer(socket, f)
		if err != nil {
			log.Print(err)
			return
		}
		defer s.Close()
		// TODO: Wait for socket to exist.
		time.Sleep(time.Second)
		c, err := newMPVClient(socket)
		if err != nil {
			log.Print(err)
			return
		}
		go c.handleOutput()
		defer c.Close()

		for f := range files {
			log.Printf("Appending %s to mpv playlist", f)
			c.appendFile(f)
		}
	}()

	m := &streamManager{tmpdir: tmpdir}
	defer m.Wait()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_ = bufferURLs(ctx, m, os.Stdin, files)
	close(files)
	defer wg.Wait()
	return nil
}

func bufferURLs(ctx context.Context, m *streamManager, r io.Reader, files chan<- string) error {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		url := scanner.Text()
		fmt.Println(url) // TODO: factor this out
		log.Printf("Buffering stream %s", url)
		if err := bufferURL(ctx, m, url, files); err != nil {
			log.Print(err)
			continue
		}
	}
	return scanner.Err()
}

func bufferURL(ctx context.Context, m *streamManager, url string, files chan<- string) error {
	path, err := m.addStream(ctx, url)
	if err != nil {
		return err
	}
	log.Printf("Got streaming FIFO %s for %s", path, url)
	files <- path
	return nil
}

// streamManager manages FIFOs created in a temporary directory and
// the streaming processes that fill the buffers feeding the FIFOs.
// Currently, this does not clean up the FIFOs, but does clean up the processes.
type streamManager struct {
	tmpdir string
	last   int
	wg     sync.WaitGroup
}

func (m *streamManager) Wait() {
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
		w.Close()
		return "", err
	}
	log.Printf("Started youtube-dl for %s to %s", url, path)
	done := make(chan struct{})
	m.wg.Add(2)
	go func() {
		_ = cmd.Wait()
		log.Printf("youtube-dl for %s exited", url)
		_ = w.Close()
		close(done)
		m.wg.Done()
	}()
	go func() {
		select {
		case <-ctx.Done():
			_ = cmd.Process.Signal(unix.SIGTERM)
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
