package ser2net

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"syscall"
	"time"

	"go.bug.st/serial.v1"
)

type Serial struct {
	SerialConn serial.Port
	path       string
	mode       serial.Mode
	connected  bool
	lastRxchar byte
	lastTxchar byte
	rxQueue    chan byte
	txQueue    chan byte

	context.Context
}

func New(ctx context.Context, port string, baud int) *Serial {
	var s Serial
	s.rxQueue = make(chan byte, 32*1024)
	s.txQueue = make(chan byte, 32*1024)
	s.path = port
	s.mode.BaudRate = baud
	s.mode.DataBits = 8
	s.mode.Parity = serial.NoParity
	s.mode.StopBits = serial.OneStopBit
	s.Context = ctx

	return &s

}

func (s *Serial) Open() error {
	p, err := serial.Open(s.path, &s.mode)
	for err != nil {
		time.Sleep(time.Second)
		p, err = serial.Open(s.path, nil)
	}
	s.connected = true
	s.SerialConn = p

	return nil
}

func (s *Serial) Serve(wg *sync.WaitGroup) {
	wg.Add(1)
	go txJob(s.Context, wg, s.txQueue, s.SerialConn)
	wg.Add(1)
	go rxJob(s.Context, wg, s.rxQueue, s.SerialConn)
}

func txJob(ctx context.Context, wg *sync.WaitGroup, txQueue chan byte, p serial.Port) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			fmt.Println("txJob terminated")

			return
		case <-txQueue:
			data := make([]byte, 0)
			for len(txQueue) > 0 {
				data = append(data, <-txQueue)
			}
			if _, err := p.Write(data); err != nil {
				fmt.Printf("error in writing from txQueue: %v", err)
			}
		}
	}
}

func rxJob(ctx context.Context, wg *sync.WaitGroup, rxQueue chan byte, p serial.Port) {
	defer wg.Done()
	for {
		select {
		case <-ctx.Done():
			fmt.Println("rxJob terminated")

			return
		default:
			b := make([]byte, 1)
			n, err := p.Read(b)

			if n > 0 {
				for i := 0; i < n; i++ {
					rxQueue <- b[i]
				}
			}
			if err != nil {
				if errors.Is(err, syscall.EINTR) || errors.Is(err, io.EOF) {
					continue
				}
			}
		}
	}
}

func (s *Serial) Read(buffer []byte) (int, error) {
	n := 0

	for {
		// Receive characters if any
		select {
		case b := <-s.rxQueue:
			if b == '\n' && s.lastRxchar != '\r' {
				if n < len(buffer) {
					buffer[n] = '\r'
					n++
				}
			}

			if n < len(buffer) {
				buffer[n] = b
				n++
			}

			s.lastRxchar = b
			if n == len(buffer) {
				return n, nil
			}

		default:
			return n, io.EOF
		}
	}
}

func (s *Serial) Write(buffer []byte) (int, error) {
	n := 0

	for _, p := range buffer {

		if s.lastTxchar == '\r' && p != '\n' {
			s.txQueue <- s.lastTxchar
			s.txQueue <- p
		}
		s.lastTxchar = p
		if p == '\n' {
			s.txQueue <- '\r'
			n++
			continue
		} else if p == 0x7f {
			s.txQueue <- '\b'
			n++
			continue
		}
		s.txQueue <- p
		n++
	}

	return n, nil
}

func (s *Serial) Close() error {
	close(s.rxQueue)
	close(s.txQueue)
	return s.SerialConn.Close()
}
