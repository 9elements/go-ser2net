package ser2net

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/PatrickRudolph/telnet"
	"github.com/PatrickRudolph/telnet/options"
	"github.com/yudai/gotty/server"
	"github.com/yudai/gotty/utils"
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

	context context.Context
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
	b := make([]byte, 16)

	// Transmit to telnet
	for {
		n, err := w.serialConn.Read(b)

		if n > 0 {
			w.mux.Lock()
			for j := 0; j < n; j++ {

				for i := range w.rxJobQueue {
					w.rxJobQueue[i] <- b[j]
				}
			}
			w.mux.Unlock()
		}

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
func (w *SerialWorker) serve(context context.Context, wr io.Writer, rr io.Reader) {
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
		p := make([]byte, 16)

		for {
			n, err := rr.Read(p)
			for j := 0; j < n; j++ {
				if p[j] == 0 {
					continue
				}
				// In binary mode there's no special CRLF handling
				// Always transmit everything received
				w.txJobQueue <- p[j]
			}
			if err != nil && strings.Contains(strings.ToLower(err.Error()), "i/o timeout") {
				time.Sleep(time.Microsecond)
				continue
			} else if err != nil {
				break
			}
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
func (w *SerialWorker) HandleTelnet(conn *telnet.Connection) {
	w.serve(context.Background(), conn, conn)
	conn.Close()
}

// Close removes the channel from the internal list
func (w *SerialWorker) Close(rx chan byte) {
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

// Open adds a channel to the internal list
func (w *SerialWorker) Open() (rx chan byte) {
	rx = make(chan byte, 4096)

	// Add RX fifo
	w.mux.Lock()
	w.rxJobQueue = append(w.rxJobQueue, rx)
	w.mux.Unlock()

	return
}

// Name returns the instance name
func (w *SerialWorker) Name() (name string) {
	name = "go-ser2net"
	return
}

// SerialIOWorker used as GoTTY factory
type SerialIOWorker struct {
	w          *SerialWorker
	rx         chan byte
	lastRxchar byte
	lastTxchar byte
}

// Read implements gotty slave interface
func (g *SerialIOWorker) Read(buffer []byte) (n int, err error) {
	var b byte

	b = <-g.rx

	for {
		//if b == '\n' && g.lastRxchar != '\r' {
		//	if n < len(buffer) {
		//		buffer[n] = '\r'
		//		n++
		//	}

		//}
		if n < len(buffer) {
			buffer[n] = b
			n++
		}

		g.lastRxchar = b
		if n == len(buffer) {
			break
		}

		// Receive more characters if any
		select {
		case b = <-g.rx:
		default:
			return
		}
	}

	return
}

// Write implements gotty slave interface
func (g *SerialIOWorker) Write(buffer []byte) (n int, err error) {

	for _, p := range buffer {

		if g.lastTxchar == '\r' && p != '\n' {
			g.w.txJobQueue <- g.lastTxchar
			g.w.txJobQueue <- p
		}
		g.lastTxchar = p
		if p == '\r' {
			g.w.txJobQueue <- '\n'
			n++
			continue
		} else if p == 0x7f {
			g.w.txJobQueue <- '\b'
			n++
			continue
		}

		g.w.txJobQueue <- p
		n++

	}

	return
}

// Close implements gotty slave interface
func (g *SerialIOWorker) Close() (err error) {
	g.w.Close(g.rx)
	return
}

// ResizeTerminal implements gotty slave interface
func (g SerialIOWorker) ResizeTerminal(columns int, rows int) (err error) {

	return
}

// WindowTitleVariables implements gotty slave interface
func (g SerialIOWorker) WindowTitleVariables() (titles map[string]interface{}) {
	titles = map[string]interface{}{
		"command": "go-ser2net",
	}
	return
}

// New returns a GoTTY slave
func (w *SerialWorker) New(params map[string][]string) (s server.Slave, err error) {
	rx := w.Open()
	s = &SerialIOWorker{w: w,
		rx: rx,
	}

	return
}

// NewIoReadWriteCloser returns a ReadWriteCloser interface
func (w *SerialWorker) NewIoReadWriteCloser() (s io.ReadWriteCloser, err error) {
	rx := w.Open()
	s = &SerialIOWorker{w: w,
		rx: rx,
	}

	return
}

// StartGoTTY starts a GoTTY server
func (w *SerialWorker) StartGoTTY(address string, port int, basicauth string) (err error) {
	htermOptions := &server.HtermPrefernces{}
	appOptions := &server.Options{
		Preferences: htermOptions,
	}
	if err = utils.ApplyDefaultValues(appOptions); err != nil {
		return
	}
	appOptions.PermitWrite = true
	appOptions.Address = address
	appOptions.EnableReconnect = true
	appOptions.Port = fmt.Sprintf("%d", port)
	appOptions.EnableBasicAuth = len(basicauth) > 0
	appOptions.Credential = basicauth
	appOptions.Preferences.BackspaceSendsBackspace = true
	hostname, _ := os.Hostname()

	appOptions.TitleVariables = map[string]interface{}{
		"command":  os.Args[0],
		"argv":     os.Args[1:],
		"hostname": hostname,
	}

	err = appOptions.Validate()
	if err != nil {
		return
	}

	srv, err := server.New(w, appOptions)
	if err != nil {
		return
	}

	err = srv.Run(w.context)

	return
}

// StartTelnet starts a telnet server
func (w *SerialWorker) StartTelnet(bindHostname string, port int) (err error) {
	svr := telnet.NewServer(fmt.Sprintf("%s:%d", bindHostname, port), w, options.EchoOption, options.SuppressGoAheadOption, options.BinaryTransmissionOption)
	return svr.ListenAndServe()
}

// NewSerialWorker creates a new SerialWorker and connect to path with 115200N8
func NewSerialWorker(context context.Context, path string, baud int) (*SerialWorker, error) {
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
	w.context = context

	return &w, nil
}
