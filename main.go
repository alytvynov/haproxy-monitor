package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/nsf/termbox-go"
	"github.com/nsf/tulib"
)

func main() {
	servers, err := parseConf("servers.conf")
	if err != nil {
		log.Fatal(err)
	}
	log.Println(servers)

	if err = termbox.Init(); err != nil {
		log.Fatal(err)
	}

	window := tulib.TermboxBuffer()
	centerLabel(&window, "termbox initialized, to exit press q")

	for {
		e := termbox.PollEvent()
		switch e.Type {
		case termbox.EventError:
			termbox.Close()
			fmt.Println(e.Err)
			return
		case termbox.EventKey:
			if e.Ch == 'q' {
				termbox.Close()
				return
			}
		}
	}
}

type server struct {
	name string
	addr string
}

func parseConf(path string) ([]server, error) {
	f, err := os.Open("servers.conf")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	servers := make([]server, 0)
	s := bufio.NewScanner(f)
	for s.Scan() {
		l := strings.Split(s.Text(), " ")
		if len(l) != 2 {
			return servers, fmt.Errorf("bad config entry %v", l)
		}
		servers = append(servers, server{name: l[0], addr: l[1]})
	}
	return servers, s.Err()
}

func centerLabel(buf *tulib.Buffer, text string) {
	x := buf.Width/2 - len(text)/2
	y := buf.Height / 2
	buf.DrawLabel(
		tulib.Rect{X: x, Y: y, Width: len(text), Height: 1},
		&tulib.LabelParams{Fg: termbox.ColorBlack, Bg: termbox.ColorWhite},
		[]byte(text),
	)
	termbox.Flush()
}
