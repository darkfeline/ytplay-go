package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"
	"sync"

	"golang.org/x/sys/unix"
)

type mpvServer struct {
	cmd      *exec.Cmd
	wait     chan struct{}
	waitOnce sync.Once
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
	return s, nil
}

func (s *mpvServer) Terminate() {
	// Not race safe.
	if _, ok := <-s.wait; ok {
		return
	}
	_ = s.cmd.Process.Signal(unix.SIGTERM)
}

func (s *mpvServer) Wait() {
	s.waitOnce.Do(func() {
		_ = s.cmd.Wait()
		log.Printf("mpv exited")
		close(s.wait)
	})
	<-s.wait
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

// appendFile appends a file to the mpv process's playlist.
func (c *mpvClient) appendFile(f string) error {
	b := []byte(fmt.Sprintf(`{"command": ["loadfile", "%s", "append-play"]}
`, f))
	_, err := c.Conn.Write(b)
	return err
}

func (c *mpvClient) handleOutput() {
	io.Copy(ioutil.Discard, c.Conn)
}
