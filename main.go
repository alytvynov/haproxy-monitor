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
	"time"

	"github.com/nsf/termbox-go"
	"github.com/nsf/tulib"
)

const fieldCount = 63

var (
	fieldPos   = []int{0, 1, 4, 8, 9, 17}
	fieldLen   = []int{17, 45, 6, 10, 10, 7}
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
		go s.monitor(view{buf, bufr, drawch, make(chan struct{})})
	}

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

func (s server) monitor(v view) {
	v.title(fmt.Sprintf("%s (%s)", s.name, s.addr))
	v.buf.Fill(tulib.Rect{Width: v.buf.Width, Height: 1, Y: v.buf.Height - 1}, termbox.Cell{Bg: termbox.ColorDefault, Fg: termbox.ColorBlue, Ch: '-'})

	for {
		s.connectAndDraw(v)
		time.Sleep(time.Second)
	}
}

func (s server) connectAndDraw(v view) {
	v.centerLabel("connecting")
	v.flush()
	time.Sleep(100 * time.Millisecond)
	con, err := net.Dial("tcp", s.addr)
	if err != nil {
		v.clearCenter()
		v.centerError("error: " + err.Error())
		v.flush()
		return
	}
	defer con.Close()

	v.clearCenter()
	v.flush()

	scan := bufio.NewScanner(con)
	buf := make([]byte, 0)
	for scan.Scan() {
		l := scan.Bytes()
		if len(l) == 0 {
			s.redraw(v, buf)
			buf = buf[:0]
			continue
		}
		buf = append(buf, append(l, '\n')...)
	}
}

func (s server) redraw(v view, stats []byte) {
	offs := 1 // offset from the top of buffer
	s.drawStatTitles(v)
	r := csv.NewReader(bytes.NewReader(stats))
	for {
		rec, err := r.Read()
		if err == io.EOF {
			break
		}
		offs++
		if err != nil {
			log.Println(err)
			v.label(offs, err.Error(), termbox.ColorRed)
			continue
		}
		if len(rec) != fieldCount {
			err := fmt.Errorf("expected %d fields, got %d", fieldCount, len(rec))
			log.Println(err)
			v.label(offs, err.Error(), termbox.ColorRed)
			continue
		}
		log.Println(rec, fieldPos)
		v.label(offs, buildLine(rec, fieldPos), termbox.ColorWhite)
	}
	v.flush()
}

func buildLine(rec []string, pos []int) string {
	l := ""
	for i, j := range pos {
		l += fmt.Sprintf(fmt.Sprintf("%%%ds|", fieldLen[i]), rec[j])
	}
	return l
}

func (s server) drawStatTitles(v view) {
	l := ""
	for i, n := range fieldNames {
		l += fmt.Sprintf(fmt.Sprintf("%%%ds|", fieldLen[i]), n)
	}
	v.label(1, l, termbox.ColorCyan)
}
