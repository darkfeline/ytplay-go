package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"
)

func main() {
	if err := innerMain(); err != nil {
		log.Fatal(err)
	}
}

func innerMain() error {
	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	cancelOnSignal(cancel)

	urls := make(chan string)
	go func() {
		sendLines(ctx, os.Stdin, urls)
		close(urls)
	}()

	// Read first URL.
	var url string
	select {
	case url = <-urls:
		fmt.Println(url)
	case <-ctx.Done():
		return ctx.Err()
	}

	tmpdir, err := ioutil.TempDir("", "ytplay")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpdir)
	log.Printf("Using tmpdir %s", tmpdir)
	m := newStreamManager(tmpdir)
	defer m.Wait()

	// Start buffering first URL.
	log.Printf("Buffering stream %s", url)
	fifo, err := m.addStream(ctx, url)
	if err != nil {
		return err
	}
	log.Printf("Got streaming FIFO %s for %s", fifo, url)

	// Start mpv and client.
	socket := filepath.Join(tmpdir, "socket")
	s, err := newMPVServer(socket, fifo)
	if err != nil {
		return err
	}
	defer s.Wait()
	defer s.Terminate()
	go func() {
		s.Wait()
		log.Printf("Canceling due to mpv exit")
		cancel()
	}()
	// TODO: Wait for socket to exist.
	time.Sleep(time.Second)
	c, err := newMPVClient(socket)
	if err != nil {
		return err
	}
	defer c.Close()
	go c.handleOutput()

	// Keep reading URLs, buffering and appending to mpv playlist.
	fifos := make(chan string)
	go func() {
		for f := range fifos {
			log.Printf("Appending %s to mpv playlist", f)
			_ = c.appendFile(f)
		}
	}()
	for url := range urls {
		fmt.Println(url)
		log.Printf("Buffering stream %s", url)
		f, err := m.addStream(ctx, url)
		if err != nil {
			log.Print(err)
			continue
		}
		log.Printf("Got streaming FIFO %s for %s", f, url)
		fifos <- f
	}
	log.Printf("Closing FIFO stream")
	close(fifos)
	return nil
}

func cancelOnSignal(cancel func()) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, unix.SIGINT, unix.SIGHUP, unix.SIGTERM)
	go func() {
		select {
		case <-c:
			log.Printf("Canceling due to signal")
			cancel()
		}
	}()
}

func sendLines(ctx context.Context, r io.Reader, lines chan<- string) error {
	pr, pw := io.Pipe()
	go io.Copy(pw, r)
	go func() {
		d := ctx.Done()
		if d == nil {
			return
		}
		<-d
		_ = pw.Close()
	}()
	scanner := bufio.NewScanner(pr)
	for scanner.Scan() {
		lines <- scanner.Text()
	}
	return scanner.Err()
}
