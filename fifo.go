package main

import (
	"bufio"
	"io"
	"io/ioutil"
	"log"
	"os"

	"golang.org/x/sys/unix"
)

// bufferedFIFO is a buffered FIFO for reading.
// This is needed because mpv needs a path to load.
// mpv also does not do prefetch well, so passing a FIFO
// directly between mpv and youtube-dl means that youtube-dl will only
// start buffering once mpv gets to it, so we have to do the buffering.
type bufferedFIFO struct {
	pw *io.PipeWriter
	pr *io.PipeReader
	bw *bufio.Writer
}

func (b *bufferedFIFO) Write(p []byte) (n int, err error) {
	return b.bw.Write(p)
}

func (b *bufferedFIFO) Close() error {
	_ = b.bw.Flush()
	return b.pw.Close()
}

// Drain unceremoniously drains the FIFO.
// Used to unblock stuff when exiting.
func (b *bufferedFIFO) Drain() {
	io.Copy(ioutil.Discard, b.pr)
}

func newBufferedFIFO(path string) (*bufferedFIFO, error) {
	if err := unix.Mkfifo(path, 0666); err != nil {
		return nil, err
	}
	r, w := io.Pipe()
	b := &bufferedFIFO{
		pw: w,
		pr: r,
		bw: bufio.NewWriterSize(w, 10*(1<<20)),
	}
	go func() {
		f, err := os.OpenFile(path, os.O_WRONLY, 0666)
		if err != nil {
			log.Print(err)
			return
		}
		defer f.Close()
		log.Printf("Writing to FIFO %s", path)
		io.Copy(f, r)
	}()
	return b, nil
}
