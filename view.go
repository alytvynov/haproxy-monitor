package main

import (
	"github.com/nsf/termbox-go"
	"github.com/nsf/tulib"
)

// draw reads view updates on in channel, blits them to termbox's back
// buffer and flushes it. This should be the only goroutine that interacts
// with termboxe's back buffer.
func draw(in chan view) {
	for v := range in {
		win := tulib.TermboxBuffer()
		win.Blit(v.rect, 0, 0, &v.buf)
		termbox.Flush()
		v.done <- struct{}{}
	}
}

type view struct {
	buf tulib.Buffer
	// rect denotes where on the window buf is located.
	// rect size is equal to buf size
	rect tulib.Rect

	draw chan view
	// done channel is used to signify update ending
	done chan struct{}
}

func (v view) flush() {
	v.draw <- v
	// wait for update to be complete so that when flush returns
	// the update is actually done
	<-v.done
}

// label draws text with foreground color col on line off
func (v view) label(off int, text string, col termbox.Attribute) {
	v.buf.DrawLabel(
		tulib.Rect{Y: off, Width: v.buf.Width, Height: 1},
		&tulib.LabelParams{Fg: col, Bg: termbox.ColorDefault},
		[]byte(text))
}

// label draws text with foreground color col on line off
func (v view) labelbg(off int, text string, bg, fg termbox.Attribute) {
	v.buf.DrawLabel(
		tulib.Rect{Y: off, Width: v.buf.Width, Height: 1},
		&tulib.LabelParams{Fg: fg, Bg: bg},
		[]byte(text))
}

// clear empties contents of everything except first line
func (v view) clear() {
	v.buf.Fill(tulib.Rect{Y: 1, Width: v.buf.Width, Height: v.buf.Height - 2},
		termbox.Cell{Bg: termbox.ColorDefault})
}

// title draws highlighted title on the first line
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

// center draws text roughly in the center of v
func (v view) center(text string, bg, fg termbox.Attribute) {
	x := v.buf.Width/2 - len(text)/2
	y := v.buf.Height / 2
	v.buf.DrawLabel(
		tulib.Rect{X: x, Y: y, Width: len(text), Height: 1},
		&tulib.LabelParams{Fg: fg, Bg: bg},
		[]byte(text),
	)
}

// clear empties entire middle line of view
func (v view) clearCenter() {
	v.buf.Fill(tulib.Rect{Y: v.buf.Height / 2, Width: v.buf.Width, Height: 1},
		termbox.Cell{Bg: termbox.ColorDefault})
}
