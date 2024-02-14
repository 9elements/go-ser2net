package ser2net_test

import (
	"bytes"
	"context"
	"sync"
	"testing"

	"github.com/9elements/go-ser2net/pkg/ser2net"
	"go.bug.st/serial.v1"
)

type PortMock struct {
	buf bytes.Buffer
}

func (s *PortMock) SetMode(mode *serial.Mode) error {
	return nil
}

func (s *PortMock) Read(p []byte) (n int, err error) {
	return s.buf.Read(p)
}

func (s *PortMock) Write(p []byte) (n int, err error) {
	return s.buf.Write(p)
}

func (s *PortMock) ResetInputBuffer() error {
	return nil
}

func (s *PortMock) ResetOutputBuffer() error {
	return nil
}

func (s *PortMock) SetDTR(dtr bool) error {
	return nil
}

func (s *PortMock) SetRTS(rts bool) error {
	return nil
}

func (s *PortMock) GetModemStatusBits() (*serial.ModemStatusBits, error) {
	return nil, nil
}

func (s *PortMock) Close() error {
	return nil
}

func TestNewSerial(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	m := &PortMock{}

	s := ser2net.New(ctx, "", 0)
	s.SerialConn = m

	var wg sync.WaitGroup
	s.Serve(&wg)

	data := "This is a test string.\rAnd another one."

	n, err := s.Write([]byte(data))
	if err != nil {
		t.Error(err)
	}

	l := len(data)

	if l != n {
		t.Error("length of input and write function mismatch")
	}

	readData := make([]byte, l)
	n, err = s.Read(readData)
	if err != nil {
		t.Error(err)
	}

	if l != n {
		t.Errorf("length of output: %d from read function: %d mismatch", l, n)
	}

	cancel()

}
