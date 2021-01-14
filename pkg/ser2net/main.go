package ser2net

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/reiver/go-telnet"
	"go.bug.st/serial.v1"
)

// SerialWorker instances one serial-network bridge
type SerialWorker struct {
	// serial connection
	serialConn serial.Port
	// serial port settings
	mode serial.Mode
	// serial port path
	path string
	// is connected
	connected bool
	// Mutex for rx handling
	mux sync.Mutex

	lastErr    string
	txJobQueue chan byte
	rxJobQueue []chan byte
}

func (w *SerialWorker) connectSerial() {
	w.connected = false

	// Poll on Serial to open (Testing)
	con, err := serial.Open(w.path, &w.mode)
	for err != nil {
		time.Sleep(time.Second)
		con, err = serial.Open(w.path, &w.mode)
	}

	w.serialConn = con
	w.connected = true

}

func (w *SerialWorker) txWorker() {
	for job := range w.txJobQueue {
		if w.connected {
			_, err := w.serialConn.Write([]byte{job})
			if err != nil {
				w.connected = false

				porterr, ok := err.(serial.PortError)
				if ok {
					fmt.Printf("ERR: Writing failed %s\n", porterr.EncodedErrorString())
					w.lastErr = porterr.EncodedErrorString()
				}
				w.serialConn.Close()
			}
		} else if job == '\n' {
			err := fmt.Sprintf("Error: %s\n", w.lastErr)
			for _, c := range []byte(err) {
				w.mux.Lock()
				for i := range w.rxJobQueue {
					w.rxJobQueue[i] <- c
				}
				w.mux.Unlock()
			}
		}
	}
}

func (w *SerialWorker) rxWorker() {
	// Transmit to telnet
	for {
		b := make([]byte, 1)
		_, err := w.serialConn.Read(b)
		fmt.Printf("|%c\n", b)

		if err != nil {
			if err == syscall.EINTR {
				continue
			}

			fmt.Printf("error reading from serial: %v\n", err)
			w.connected = false

			porterr, ok := err.(serial.PortError)
			if ok {
				fmt.Printf("ERR: Reading failed %s\n", porterr.EncodedErrorString())
				w.lastErr = porterr.EncodedErrorString()
			}
			w.serialConn.Close()
			break
		}

		w.mux.Lock()
		for i := range w.rxJobQueue {
			w.rxJobQueue[i] <- b[0]
		}
		w.mux.Unlock()
	}
}

// Worker is the worker operating the serial port. Never returns.
func (w *SerialWorker) Worker() {
	// Receive from telnet
	go w.txWorker()
	for {
		w.connectSerial()

		// Transmit to telnet
		go w.rxWorker()

		_, err := os.Stat(w.path)
		for err == nil {
			time.Sleep(time.Second)
			_, err = os.Stat(w.path)
		}
		w.serialConn.Close()
	}
}

// Serve is invoked by an external entity to provide a Reader and Writer interface
func (w *SerialWorker) Serve(context context.Context, wr io.Writer, rr io.Reader) {
	var wg sync.WaitGroup
	wg.Add(2)

	rx := make(chan byte, 4096)

	// Add RX fifo
	w.mux.Lock()
	w.rxJobQueue = append(w.rxJobQueue, rx)
	w.mux.Unlock()

	go func() {
		var lastchar byte

		for b := range rx {
			if b == '\n' && lastchar != '\r' {

				_, err := wr.Write([]byte{'\r'})
				if err != nil {
					break
				}
			}
			_, err := wr.Write([]byte{b})
			if err != nil {
				break
			}
			lastchar = b
		}
		wg.Done()
	}()
	go func() {
		var lastchar byte
		var buffer [1]byte // Seems like the length of the buffer needs to be small, otherwise will have to wait for buffer to fill up.
		p := buffer[:]
		for {
			_, err := rr.Read(p)
			if err != nil {
				break
			}

			if lastchar == '\r' && p[0] != '\n' {
				w.txJobQueue <- lastchar
				w.txJobQueue <- p[0]
			}

			lastchar = p[0]
			if p[0] == '\r' {
				continue
			}
			w.txJobQueue <- p[0]

		}
		wg.Done()
	}()

	wg.Wait()

	// Remove RX fifo
	w.mux.Lock()
	var new []chan byte

	for i := range w.rxJobQueue {
		if w.rxJobQueue[i] != rx {
			new = append(new, w.rxJobQueue[i])
		}
	}
	w.rxJobQueue = new
	w.mux.Unlock()
}

// ServeTELNET is the worker operating the telnet port - used by reiver/go-telnet
func (w *SerialWorker) ServeTELNET(telnetContext telnet.Context, wr telnet.Writer, rr telnet.Reader) {

	// Disable local echo on client
	_, err := wr.Write([]byte{0xFF, 0xFB, 0x01}) // IAC WILL ECHO
	if err != nil {
		return
	}

	// Disable local echo on client
	_, err = wr.Write([]byte{0xFF, 0xFB, 0x03}) // IAC WILL SUPRESS GO AHEAD
	if err != nil {
		return
	}

	w.Serve(context.Background(), wr, rr)
}

// NewSerialWorker creates a new SerialWorker and connect to path with 115200N8
func NewSerialWorker(path string, baud int) (*SerialWorker, error) {
	var w SerialWorker
	w.txJobQueue = make(chan byte, 4096)
	if baud <= 0 {
		baud = 115200
	}
	w.mode.BaudRate = baud
	w.mode.DataBits = 8
	w.mode.Parity = serial.NoParity
	w.mode.StopBits = serial.OneStopBit
	w.path = path
	w.connected = false
	w.lastErr = "Serial is not connected"

	return &w, nil
}
