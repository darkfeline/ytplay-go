package main

import (
	"bufio"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
)

func readInput(c chan<- string) {
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

func stream(in <-chan string, out chan<- io.ReadCloser) {
	defer close(out)
	for video := range in {
		cmd := exec.Command("youtube-dl", "-q", "-o", "-", video)
		stream, err := cmd.StdoutPipe()
		if err != nil {
			log.Print(err)
			break
		}
		log.Printf("Streaming %s", video)
		cmd.Start()
		out <- stream
	}
}

func proxy(streams <-chan io.ReadCloser) {
	for stream := range streams {
		cmd := exec.Command("mpv", "--no-terminal", "--no-video", "-")
		cmd.Stdin = stream
		cmd.Run()
	}
}

func main() {
	videos := make(chan string)
	streams := make(chan io.ReadCloser)

	go readInput(videos)
	go stream(videos, streams)
	proxy(streams)
}
