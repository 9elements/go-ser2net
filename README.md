go-ser2net
========================

A serial to telnet client and library as replacement for ser2net.


# How to use the library

To spawn a telnet server on port 5555 and redirect to ttyS0 running at 115200N8 use this code
example:

```
import (
	"github.com/9elements/go-ser2net/pkg/ser2net"
	"github.com/reiver/go-telnet"
)

        w, _ := ser2net.NewSerialWorker("/dev/ttyS0")
	// Run serial worker in new routine
        go w.Worker()

	// Start telnet on port 5555 and serve forever
        err := w.StartTelnet("", 5555)
        if nil != err {
                panic(err)
        }

	// Start GoTTY on port 5556 and serve forever
	err = w.StartGoTTY("", 5556, "")
        if nil != err {
                panic(err)
        }
```
