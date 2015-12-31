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
		c <- video
	}
	if err := stdin.Err(); err != nil {
		log.Fatal(err)
	}
}

func stream(in <-chan string, out chan<- io.ReadCloser) {
	defer close(out)
	for video := range in {
		cmd := exec.Command("youtube-dl", "-q", "-o", "-", video)
		stream, err := cmd.StdoutPipe()
		if err != nil {
			log.Fatal(err)
		}
		cmd.Start()
		out <- stream
	}
}

func proxy(playerIn io.WriteCloser, streams <-chan io.ReadCloser) {
	defer playerIn.Close()
	buffer := make([]byte, 1024)
	for stream := range streams {
		proxyOne(buffer, playerIn, stream)
	}
}

func proxyOne(buffer []byte, playerIn io.WriteCloser, stream io.ReadCloser) {
	for {
		n, err := stream.Read(buffer)
		_, err2 := playerIn.Write(buffer[:n])
		if err2 != nil {
			log.Fatal(err2)
		}
		switch err {
		case nil:
		case io.EOF:
			return
		default:
			log.Fatal(err)
		}
	}
}

func main() {
	player := exec.Command("mpv", "--no-terminal", "--no-video", "-")
	playerIn, err := player.StdinPipe()
	if err != nil {
		log.Fatal(err)
	}
	videos := make(chan string)
	streams := make(chan io.ReadCloser)

	go readInput(videos)
	go stream(videos, streams)
	go proxy(playerIn, streams)
	player.Start()

	player.Wait()
}
