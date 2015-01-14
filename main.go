package main

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/nsf/termbox-go"
	"github.com/nsf/tulib"
)

const fieldCount = 63

var (
	fieldPos   = []int{0, 1, 4, 8, 9, 17}
	fieldLen   = []int{23, 35, 6, 10, 10, 7}
	fieldNames = []string{"group", "name", "scur", "bin", "bout", "status"}
)

func main() {
	logf, err := os.Create("monitor.log")
	if err != nil {
		log.Fatal(err)
	}
	defer logf.Close()
	log.SetOutput(logf)
	servers, err := parseConf("servers.conf")
	if err != nil {
		log.Fatal(err)
	}
	log.Println(servers)

	if err = termbox.Init(); err != nil {
		log.Fatal(err)
	}

	drawch := make(chan view)
	go draw(drawch)

	w, h := termbox.Size()
	bh := h / len(servers)
	for i, s := range servers {
		buf := tulib.NewBuffer(w, bh)
		bufr := tulib.Rect{X: 0, Y: bh * i, Width: w, Height: bh}
		s.v = view{buf, bufr, drawch, make(chan struct{})}
		go s.monitor()
	}

	sels := 0

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
				for !servers[sels].moveCursor(-1) {
					if sels == 0 {
						break
					}
					sels--
				}
			case termbox.KeyArrowDown:
				for !servers[sels].moveCursor(1) {
					if sels == len(servers)-1 {
						break
					}
					sels++
				}
			}
			switch e.Ch {
			case 'q':
				termbox.Close()
				return
			}
		}
	}
}

func draw(in chan view) {
	for v := range in {
		win := tulib.TermboxBuffer()
		win.Blit(v.rect, 0, 0, &v.buf)
		termbox.Flush()
		v.done <- struct{}{}
	}
}

type view struct {
	buf  tulib.Buffer
	rect tulib.Rect

	draw chan view
	done chan struct{}
}

func (v view) flush() {
	v.draw <- v
	<-v.done
}

func (v view) label(off int, text string, col termbox.Attribute) {
	v.buf.DrawLabel(
		tulib.Rect{Y: off, Width: v.buf.Width, Height: 1},
		&tulib.LabelParams{Fg: col, Bg: termbox.ColorDefault},
		[]byte(text))
}

func (v view) clear() {
	v.buf.Fill(tulib.Rect{Y: 1, Width: v.buf.Width, Height: v.buf.Height - 2},
		termbox.Cell{Bg: termbox.ColorDefault})
}

func (v view) title(text string) {
	v.buf.DrawLabel(
		tulib.Rect{Width: v.buf.Width, Height: 1},
		&tulib.LabelParams{Fg: termbox.ColorBlack, Bg: termbox.ColorWhite},
		[]byte(text))
}

func (v view) centerLabel(text string) {
	v.center(text, termbox.ColorWhite, termbox.ColorBlack)
}

func (v view) centerError(text string) {
	v.center(text, termbox.ColorWhite, termbox.ColorRed)
}

func (v view) center(text string, bg, fg termbox.Attribute) {
	x := v.buf.Width/2 - len(text)/2
	y := v.buf.Height / 2
	v.buf.DrawLabel(
		tulib.Rect{X: x, Y: y, Width: len(text), Height: 1},
		&tulib.LabelParams{Fg: fg, Bg: bg},
		[]byte(text),
	)
}

func (v view) clearCenter() {
	v.buf.Fill(tulib.Rect{Y: v.buf.Height / 2, Width: v.buf.Width, Height: 1},
		termbox.Cell{Bg: termbox.ColorDefault})
}

type server struct {
	name string
	addr string

	v          view
	numr, selr int
	curRec     []byte
	mu         sync.Mutex
}

func parseConf(path string) ([]*server, error) {
	f, err := os.Open("servers.conf")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	servers := make([]*server, 0)
	s := bufio.NewScanner(f)
	for s.Scan() {
		l := strings.Split(s.Text(), " ")
		if len(l) != 2 {
			return servers, fmt.Errorf("bad config entry %v", l)
		}
		servers = append(servers, &server{name: l[0], addr: l[1]})
	}
	return servers, s.Err()
}

func (s *server) monitor() {
	s.selr = -1
	s.v.title(fmt.Sprintf("%s (%s)", s.name, s.addr))
	s.v.buf.Fill(tulib.Rect{Width: s.v.buf.Width, Height: 1, Y: s.v.buf.Height - 1}, termbox.Cell{Bg: termbox.ColorDefault, Fg: termbox.ColorBlue, Ch: '-'})

	for {
		s.connectAndDraw()
		time.Sleep(time.Second)
	}
}

func (s *server) connectAndDraw() {
	s.v.clearCenter()
	s.v.centerLabel("connecting")
	s.v.flush()
	time.Sleep(100 * time.Millisecond)
	con, err := net.Dial("tcp", s.addr)
	if err != nil {
		s.v.clearCenter()
		s.v.centerError("error: " + err.Error())
		s.v.flush()
		return
	}
	defer con.Close()

	s.v.clearCenter()
	s.v.flush()

	scan := bufio.NewScanner(con)
	buf := make([]byte, 0)
	for scan.Scan() {
		l := scan.Bytes()
		if len(l) == 0 {
			s.curRec = buf
			s.redraw()
			buf = buf[:0]
			continue
		}
		buf = append(buf, append(l, '\n')...)
	}
}

func (s *server) redraw() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.drawStatTitles()

	offs := 1 // offset from the top of buffer
	r := csv.NewReader(bytes.NewReader(s.curRec))
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		offs++
		if err != nil {
			log.Println(err)
			s.v.label(offs, err.Error(), termbox.ColorRed)
			continue
		}
		if len(rec) != fieldCount {
			err := fmt.Errorf("expected %d fields, got %d", fieldCount, len(rec))
			log.Println(err)
			s.v.label(offs, err.Error(), termbox.ColorRed)
			continue
		}
		s.appendLine(offs, rec)
	}
	s.numr = offs - 1
	s.v.flush()
}

func (s *server) appendLine(offs int, rec []string) {
	l := ""
	for i, j := range fieldPos {
		l += fmt.Sprintf("%*.*s |", fieldLen[i], fieldLen[i], rec[j])
	}
	if s.selr == offs-2 {
		s.v.label(offs, l, termbox.ColorYellow)
	} else {
		s.v.label(offs, l, termbox.ColorWhite)
	}
}

func (s *server) drawStatTitles() {
	l := ""
	for i, n := range fieldNames {
		l += fmt.Sprintf("%*.*s|", fieldLen[i], fieldLen[i], n)
	}
	s.v.label(1, l, termbox.ColorCyan)
}

func (s *server) moveCursor(diff int) (res bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.selr += diff
	switch {
	case s.selr >= s.numr:
		s.selr = s.numr
	case s.selr < 0:
		s.selr = -1
	default:
		res = true
	}
	log.Println(s.name, "move", diff, s.selr, res)
	go s.redraw()
	return res
}
