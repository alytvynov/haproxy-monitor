package main

import (
	"fmt"
	"log"
	"os"

	"github.com/nsf/termbox-go"
)

func main() {
	// set log output to a file so we don't screw up the interface
	logf, err := os.Create("monitor.log")
	if err != nil {
		log.Fatal(err)
	}
	defer logf.Close()
	log.SetOutput(logf)

	if err = termbox.Init(); err != nil {
		log.Fatal(err)
	}

	servers, err := setupServers()
	if err != nil {
		termbox.Close()
		log.Fatal(err)
	}
	log.Println(servers)

	pollEvents(servers)
}

// pollEvents listens for user input and forwards commands to views as
// needed
func pollEvents(servers []*server) {
	sels := 0 // index of currently selected server
	for {
		e := termbox.PollEvent()
		switch e.Type {
		case termbox.EventError:
			termbox.Close()
			fmt.Println(e.Err)
			return
		case termbox.EventKey:
			switch e.Key {
			case termbox.KeyArrowUp:
				// go trough all servers until first trying to move the cursor
				for !servers[sels].moveCursor(-1) {
					if sels == 0 {
						break
					}
					sels--
				}
			case termbox.KeyArrowDown:
				// go trough all servers until last trying to move the cursor
				for !servers[sels].moveCursor(1) {
					if sels == len(servers)-1 {
						break
					}
					sels++
				}
			}
			switch e.Ch {
			case 'q': // q means quit
				termbox.Close()
				return
			case 'm':
				servers[sels].toggleSelected()
			}
		}
	}
}
