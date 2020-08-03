package main

import (
	"flag"
	"fmt"

	"github.com/9elements/go-ser2net/pkg/ser2net"
	"github.com/reiver/go-telnet"
)

func main() {
	port := 1234
	devPath := ""
	bindHostname := ""

	flag.StringVar(&bindHostname, "bind", "", "Hostname or IP to bind telnet to")
	flag.StringVar(&devPath, "dev", "", "TTY to open")
	flag.IntVar(&port, "port", 0, "Telnet port")

	flag.Parse()
	if devPath == "" {
		flag.Usage()
		panic("Error: Device path not set")
	}

	w, _ := ser2net.NewSerialWorker(devPath)
	go w.Worker()

	err := telnet.ListenAndServe(fmt.Sprintf("%s:%d", bindHostname, port), w)
	if nil != err {
		panic(err)
	}
}
