package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
)

type streamBuffer struct {
	stream io.ReadCloser
	url    string
}

func newStreamBuffer(stream io.ReadCloser, url string) *streamBuffer {
	return &streamBuffer{stream, url}
}

// Read input URLS from stdin.
func reader(c chan<- string) {
	defer close(c)
	stdin := bufio.NewScanner(os.Stdin)
	for stdin.Scan() {
		video := stdin.Text()
		fmt.Println(video)
		log.Printf("Read %s", video)
		c <- video
	}
	if err := stdin.Err(); err != nil {
		log.Print(err)
	}
}

// Buffer video streams for input URLs.
func bufferer(in <-chan string, out chan<- *streamBuffer) {
	defer close(out)
	for video := range in {
		cmd := exec.Command("youtube-dl", "-q", "-o", "-", video)
		stream, err := cmd.StdoutPipe()
		if err != nil {
			log.Print(err)
			break
		}
		log.Printf("Buffering %s", video)
		cmd.Start()
		out <- newStreamBuffer(stream, video)
	}
}

// Play buffered streams one by one as they come in.
func player(streams <-chan *streamBuffer) {
	for stream := range streams {
		cmd := exec.Command("mpv", "--no-terminal", "--no-video", "-")
		cmd.Stdin = stream.stream
		log.Printf("Playing %s", stream.url)
		cmd.Run()
	}
}

func main() {
	videos := make(chan string)
	streams := make(chan *streamBuffer)

	go reader(videos)
	go bufferer(videos, streams)
	player(streams)
}
