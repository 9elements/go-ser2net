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
        go w.Worker()

        err := telnet.ListenAndServe(":5555", w)
        if nil != err {
                panic(err)
        }
```
