package ser2net

import (
	"fmt"
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

// ServeTELNET is the worker operating the telnet port
func (w *SerialWorker) ServeTELNET(telnetContext telnet.Context, wr telnet.Writer, rr telnet.Reader) {
	var wg sync.WaitGroup
	wg.Add(2)

	var buffer [1]byte // Seems like the length of the buffer needs to be small, otherwise will have to wait for buffer to fill up.
	p := buffer[:]

	rx := make(chan byte, 4096)

	// Add RX fifo

	w.mux.Lock()
	w.rxJobQueue = append(w.rxJobQueue, rx)
	w.mux.Unlock()

	go func() {
		for b := range rx {
			_, err := wr.Write([]byte{b})
			if err != nil {
				break
			}
		}
		wg.Done()
	}()
	go func() {
		for {
			_, err := rr.Read(p)
			if err != nil {
				break
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
