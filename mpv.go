package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net"
	"os"
	"os/exec"

	"golang.org/x/sys/unix"
)

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
