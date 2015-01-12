package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"time"
)

func main() {
	reconnect(os.Args[1])
}

func reconnect(addr string) {
	for {
		log.Println("connecting to", addr)
		if err := poll(addr); err != nil {
			log.Println(err)
		}
	}
}

func poll(addr string) error {
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

	log.Println("> prompt")
	time.Sleep(time.Second)

	s := bufio.NewScanner(con)
	for {
		_, err = con.Write([]byte("show stat\n"))
		if err != nil {
			return err
		}
		log.Println("> show stat")

	printLoop:
		for s.Scan() {
			l := s.Text()
			switch {
			case l == "": // empty line is end of stat output
				break printLoop
			case strings.HasPrefix(l, ">"): // first line comment
			case strings.HasPrefix(l, "stats"): // stats backends
			default:
				fmt.Println("<", l)
			}
		}

		if s.Err() != nil {
			return s.Err()
		}

		time.Sleep(time.Second)
	}
}
