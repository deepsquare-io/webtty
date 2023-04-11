//go:build windows
// +build windows

package main

import (
	"fmt"
	"log"
	"os"
	"time"

	"github.com/creack/pty"
	"github.com/mitchellh/colorstring"
	"golang.org/x/sys/windows"
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

		go cs.monitorTerminalResize()

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

func (cs *clientSession) monitorTerminalResize() {
	var lastWinSize windows.ConsoleScreenBufferInfo
	var currentWinSize windows.ConsoleScreenBufferInfo

	hConsole, err := windows.GetStdHandle(windows.STD_INPUT_HANDLE)
	if err != nil {
		log.Println("Error getting console handle:", err)
		cs.errChan <- err
		return
	}

	for {
		// Get the current terminal size
		err := windows.GetConsoleScreenBufferInfo(hConsole, &currentWinSize)
		if err != nil {
			log.Println("Error getting console screen buffer info:", err)
			cs.errChan <- err
			return
		}

		// Check if terminal size has changed
		if lastWinSize.Window.Right != currentWinSize.Window.Right || lastWinSize.Window.Bottom != currentWinSize.Window.Bottom {
			err := sendTermSize(os.Stdin, cs.dc.SendText)
			if err != nil {
				log.Println(err)
				cs.errChan <- err
			}
			lastWinSize = currentWinSize
		}

		time.Sleep(250 * time.Millisecond)
	}
}
