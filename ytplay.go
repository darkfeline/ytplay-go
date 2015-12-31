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

func bufferer(in <-chan string, out chan<- streamBuffer) {
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
		out <- streamBuffer{stream, video}
	}
}

func player(streams <-chan streamBuffer) {
	for stream := range streams {
		cmd := exec.Command("mpv", "--no-terminal", "--no-video", "-")
		cmd.Stdin = stream.stream
		log.Printf("Playing %s", stream.url)
		cmd.Run()
	}
}

func main() {
	videos := make(chan string)
	streams := make(chan streamBuffer)

	go reader(videos)
	go bufferer(videos, streams)
	player(streams)
}
