//go:build !windows
// +build !windows

package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/creack/pty"
	"github.com/mitchellh/colorstring"
)

func sendTermSize(term *os.File, dcSend func(s string) error) error {
	winSize, err := pty.GetsizeFull(term)
	if err != nil {
		log.Fatal(err)
	}
	size := fmt.Sprintf(`["set_size",%d,%d,%d,%d]`,
		winSize.Rows, winSize.Cols, winSize.X, winSize.Y)

	return dcSend(size)
}

func (cs *clientSession) dataChannelOnOpen() func() {
	return func() {
		log.Printf("Data channel '%s'-'%d'='%d' open.\n", cs.dc.Label(), cs.dc.ID(), cs.dc.MaxPacketLifeTime())
		colorstring.Println("[bold]Terminal session started:")

		if err := cs.makeRawTerminal(); err != nil {
			log.Println(err)
			cs.errChan <- err
		}

		ch := make(chan os.Signal, 1)
		signal.Notify(ch, syscall.SIGWINCH)
		go func() {
			for range ch {
				err := sendTermSize(os.Stdin, cs.dc.SendText)
				if err != nil {
					log.Println(err)
					cs.errChan <- err
				}
			}
		}()
		ch <- syscall.SIGWINCH // Initial resize.
		buf := make([]byte, 1024)
		for {
			nr, err := os.Stdin.Read(buf)
			if err != nil {
				log.Println(err)
				cs.errChan <- err
			}
			err = cs.dc.Send(buf[0:nr])
			if err != nil {
				log.Println(err)
				cs.errChan <- err
			}
		}
	}
}
