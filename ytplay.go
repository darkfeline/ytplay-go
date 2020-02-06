package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"sync"

	"golang.org/x/sys/unix"
)

var (
	video = flag.Bool("video", false, "Enable video")
)

func main() {
	flag.Parse()
	streams := make(chan io.Reader, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		scanner := bufio.NewScanner(os.Stdin)
		for scanner.Scan() {
			url := scanner.Text()
			fmt.Println(url)
			r, err := bufferStream(ctx, url)
			if err != nil {
				log.Print(err)
				continue
			}
			streams <- r
		}
		if err := scanner.Err(); err != nil {
			log.Print(err)
		}
		close(streams)
		wg.Done()
	}()
	go func() {
		for r := range streams {
			playStream(r)
		}
		wg.Done()
	}()
	wg.Wait()
}

func bufferStream(ctx context.Context, url string) (io.Reader, error) {
	cmd := exec.Command("youtube-dl", "-q", "-o", "-", url)
	cmd.Stderr = os.Stderr
	r, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	go func() {
		select {
		case <-ctx.Done():
			_ = cmd.Process.Signal(unix.SIGTERM)
		case <-done:
		}
	}()
	return r, nil
}

func playStream(r io.Reader) {
	var cmd *exec.Cmd
	if *video {
		cmd = exec.Command("mpv", "--no-terminal", "-")

	} else {
		cmd = exec.Command("mpv", "--no-terminal", "--no-video", "-")
	}
	cmd.Stdin = r
	_ = cmd.Run()
}
