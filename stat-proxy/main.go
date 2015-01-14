package main

import (
	"bufio"
	"bytes"
	"flag"
	"log"
	"net"
	"time"
)

var laddr = flag.String("laddr", ":8081", "address to listen on")

func main() {
	flag.Parse()

	datach := make(chan []byte)
	cmdch := make(chan []byte)
	go reconnect(flag.Arg(0), cmdch, datach)
	subch := make(chan subscriber)
	go pubsub(datach, subch)

	l, err := net.Listen("tcp", *laddr)
	if err != nil {
		log.Fatal(err)
	}

	for {
		con, err := l.Accept()
		if err != nil {
			log.Println(err)
			continue
		}

		go serve(con, subch, cmdch)
	}
}

func reconnect(addr string, in, out chan []byte) {
	for {
		log.Println("connecting to", addr)
		if err := poll(addr, in, out); err != nil {
			log.Println(err)
		}
		time.Sleep(time.Second)
	}
}

func poll(addr string, in, out chan []byte) error {
	uaddr, err := net.ResolveUnixAddr("unix", addr)
	if err != nil {
		return err
	}

	con, err := net.DialUnix("unix", nil, uaddr)
	if err != nil {
		return err
	}
	defer con.Close()

	_, err = con.Write([]byte("prompt\n"))
	if err != nil {
		return err
	}

	time.Sleep(time.Second)

	buf := &bytes.Buffer{}

	s := bufio.NewScanner(con)
	for {
		select {
		case cmd := <-in:
			_, err = con.Write(append(cmd, '\n'))
			if err != nil {
				return err
			}
			continue
		default:
		}
		_, err = con.Write([]byte("show stat\n"))
		if err != nil {
			return err
		}

	printLoop:
		for s.Scan() {
			l := s.Bytes()
			switch {
			case len(l) == 0: // empty line is end of stat output
				break printLoop
			case bytes.HasPrefix(l, []byte(">")): // first line comment
			case bytes.HasPrefix(l, []byte("stats")): // stats backends
			default:
				buf.Write(append(l, '\n'))
			}
		}
		buf.Write([]byte{'\n'})
		out <- buf.Bytes()
		buf.Reset()

		if s.Err() != nil {
			return s.Err()
		}

		time.Sleep(time.Second)
	}
}

func serve(con net.Conn, sub chan subscriber, cmd chan []byte) {
	defer con.Close()

	log.Println("new subscriber", con.RemoteAddr())
	defer log.Println("subscriber disconnected", con.RemoteAddr())

	in := make(chan []byte)
	sub <- subscriber{id: con.RemoteAddr().String(), ch: in}
	defer func() {
		sub <- subscriber{id: con.RemoteAddr().String(), unsub: true}
	}()

	go func() {
		s := bufio.NewScanner(con)
		for s.Scan() {
			log.Println("command:", s.Text())
			cmd <- s.Bytes()
		}
		if s.Err() != nil {
			log.Println(s.Err())
		}
	}()
	for stat := range in {
		_, err := con.Write(stat)
		if err != nil {
			log.Println(err)
			return
		}
	}
}

type subscriber struct {
	id    string
	unsub bool
	ch    chan []byte
}

func pubsub(in chan []byte, sub chan subscriber) {
	var data []byte
	subs := make(map[string]chan []byte)

	for {
		select {
		case data = <-in:
			for _, s := range subs {
				select {
				case s <- data:
				default:
				}
			}
		case s := <-sub:
			if s.unsub {
				delete(subs, s.id)
			} else {
				subs[s.id] = s.ch
			}
			log.Println("len(subs):", len(subs))
		}
	}
}
