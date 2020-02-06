package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
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
			log.Printf("Appending mpv file %s", f)
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

type mpvServer struct {
	cmd  *exec.Cmd
	wait chan struct{}
}

func newMPVServer(socket string, file string) (*mpvServer, error) {
	args := []string{"mpv", "--no-config", "--no-terminal", "--force-window=immediate",
		"--cache=yes", "--cache-secs=600", "--keep-open=yes",
		"--input-ipc-server=" + socket, file}
	log.Printf("Starting mpv with %v", args)
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	s := &mpvServer{
		cmd:  cmd,
		wait: make(chan struct{}),
	}
	go func() {
		_ = s.cmd.Wait()
		log.Printf("mpv exited")
		close(s.wait)
	}()
	return s, nil
}

func (s *mpvServer) Close() error {
	_ = s.cmd.Process.Signal(unix.SIGTERM)
	<-s.wait
	return nil
}

type mpvClient struct {
	net.Conn
}

func newMPVClient(socket string) (*mpvClient, error) {
	conn, err := net.Dial("unix", socket)
	if err != nil {
		return nil, err
	}
	return &mpvClient{
		Conn: conn,
	}, nil
}

func (c *mpvClient) appendFile(f string) {
	b := []byte(fmt.Sprintf(`{"command": ["loadfile", "%s", "append-play"]}
`, f))
	_, _ = c.Conn.Write(b)
}

func (c *mpvClient) handleOutput() {
	io.Copy(ioutil.Discard, c.Conn)
}

func bufferURLs(ctx context.Context, m *streamManager, r io.Reader, files chan<- string) error {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		url := scanner.Text()
		fmt.Println(url) // TODO: factor this out
		log.Printf("Buffering stream %s", url)
		path, err := m.addStream(ctx, url)
		if err != nil {
			log.Print(err)
			continue
		}
		log.Printf("Got streaming FIFO %s for %s", path, url)
		files <- path
	}
	return scanner.Err()
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
