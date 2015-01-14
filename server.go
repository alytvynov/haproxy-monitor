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

// fieldCount is expected number of stat elements per record
const fieldCount = 63

var (
	// fieldPos are positions of stats that we want to print
	fieldPos = []int{0, 1, 4, 8, 9, 17}
	// fieldLen contains widths of columns drawn for respective fields.
	// len(fieldPos) == len(fieldLen)
	fieldLen = []int{23, 35, 6, 10, 10, 7}
	// fieldNames are column names to be drawn
	// len(fieldPos) == len(fieldNames)
	fieldNames = []string{"group", "name", "scur", "bin", "bout", "status"}
)

// server is a top-level struct that controls it's own section of the
// screen defined by v field, polls it's HAProxy server and refreshes the
// state
type server struct {
	name string
	addr string
	con  net.Conn

	// guards recs, selr and v
	mu sync.Mutex
	v  view
	// number of fields and currently selected field
	selr int
	// current stat state, unparsed cvs values
	stat []byte
	// current records
	recs [][]string
}

func setupServers() ([]*server, error) {
	servers, err := parseConf("servers.conf")
	if err != nil {
		return nil, err
	}

	// drawch is used by views to request redrawing them
	drawch := make(chan view)
	go draw(drawch)

	w, h := termbox.Size()
	bh := h / len(servers) // height of each server's view
	for i, s := range servers {
		buf := tulib.NewBuffer(w, bh)
		bufr := tulib.Rect{X: 0, Y: bh * i, Width: w, Height: bh}
		s.v = view{buf, bufr, drawch, make(chan struct{})}
		go s.monitor()
	}

	return servers, nil
}

// parseConf parses named file and extracts info about servers
// config format is: name address
// one entry per line
func parseConf(path string) ([]*server, error) {
	f, err := os.Open(path)
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

// monitor sets up title and manages reconnection
func (s *server) monitor() {
	s.selr = -1
	s.v.title(fmt.Sprintf("%s (%s)", s.name, s.addr))
	s.v.buf.Fill(tulib.Rect{Width: s.v.buf.Width, Height: 1, Y: s.v.buf.Height - 1}, termbox.Cell{Bg: termbox.ColorDefault, Fg: termbox.ColorBlue, Ch: '-'})

	for {
		s.connectAndDraw()
		time.Sleep(time.Second)
	}
}

// connectAndDraw reads data from the server and refreshes the interface
func (s *server) connectAndDraw() {
	s.v.clearCenter()
	s.v.centerLabel("connecting")
	s.v.flush()
	time.Sleep(100 * time.Millisecond)
	var err error
	s.con, err = net.Dial("tcp", s.addr)
	if err != nil {
		s.v.clearCenter()
		s.v.centerError("error: " + err.Error())
		s.v.flush()
		return
	}
	defer s.con.Close()

	s.v.clearCenter()
	s.v.flush()

	scan := bufio.NewScanner(s.con)
	buf := make([]byte, 0)
	for scan.Scan() {
		l := scan.Bytes()
		// if it's an empty line we have a full batch of stats, trigger redraw
		if len(l) == 0 {
			//copy buf to avoid races
			cp := make([]byte, len(buf))
			copy(cp, buf)

			// lock here in case we have concurrent redraw happening that
			// reads s.stat
			s.mu.Lock()
			s.stat = cp
			s.mu.Unlock()

			s.redraw()
			buf = buf[:0]
			continue
		}
		buf = append(buf, append(l, '\n')...)
	}
}

func (s *server) redraw() {
	// protect cursor position fields and v
	s.mu.Lock()
	defer s.mu.Unlock()

	s.drawStatTitles()

	s.recs = s.recs[:0]
	offs := 1 // offset from the top of buffer
	r := csv.NewReader(bytes.NewReader(s.stat))
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
		// skip those extra records that are not actual servers
		if rec[1] == "BACKEND" || rec[1] == "FRONTEND" {
			continue
		}

		s.recs = append(s.recs, rec)
		s.appendLine(offs, rec)
	}
	s.v.flush()
}

func (s *server) appendLine(offs int, rec []string) {
	l := ""
	for i, j := range fieldPos {
		// *.* uses arguments for size limiting
		l += fmt.Sprintf("%*.*s | ", fieldLen[i], fieldLen[i], rec[j])
	}
	// since offs starts at 2
	if s.selr == offs-2 {
		s.v.labelbg(offs, l, termbox.ColorWhite, termbox.ColorBlack) // selected line
	} else {
		s.v.label(offs, l, termbox.ColorWhite) //regular line
	}
}

func (s *server) drawStatTitles() {
	l := ""
	for i, n := range fieldNames {
		l += fmt.Sprintf("%*.*s | ", fieldLen[i], fieldLen[i], n)
	}
	s.v.label(1, l, termbox.ColorCyan)
}

// move cursor by diff. Positive diff goes lower on the list, negative - higher.
// If res is false cursor moved out of bounds and is no longer visible
func (s *server) moveCursor(diff int) (res bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.selr += diff
	switch {
	case s.selr >= len(s.recs):
		// if we went below the list, keep the cursor one position below last element
		s.selr = len(s.recs)
	case s.selr < 0:
		// if we went above the list, keep the cursor one position above first element
		s.selr = -1
	default:
		res = true
	}

	// trigger redraw. Necessary to in a goroutine because of locking
	go s.redraw()
	return res
}

func (s *server) toggleSelected() {
	s.mu.Lock()
	s.mu.Unlock()

	if s.selr < 0 || s.selr > len(s.recs)-1 {
		return
	}
	action := "disable"
	if s.recs[s.selr][17] != "UP" {
		action = "enable"
	}
	if _, err := fmt.Fprintf(s.con, "%s server %s/%s\n", action, s.recs[s.selr][0], s.recs[s.selr][1]); err != nil {
		log.Println(err)
	}
}
