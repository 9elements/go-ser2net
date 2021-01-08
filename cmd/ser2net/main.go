package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/9elements/go-ser2net/pkg/ser2net"
	"github.com/reiver/go-telnet"
)

func main() {
	port := 1234
	devPath := ""
	configPath := ""
	bindHostname := ""

	flag.StringVar(&bindHostname, "bind", "", "Hostname or IP to bind telnet to")
	flag.StringVar(&devPath, "dev", "", "TTY to open")
	flag.StringVar(&configPath, "config", "", "TTY to open")
	flag.IntVar(&port, "port", 0, "Telnet port")

	flag.Parse()
	if devPath == "" && configPath == "" {
		flag.Usage()
		panic("Error: Device path not set and config not given")
	}

	if configPath != "" {
		var wg sync.WaitGroup

		file, err := os.Open(configPath)
		if err != nil {
			log.Fatal(err)
		}
		defer file.Close()

		scanner := bufio.NewScanner(file)
		for scanner.Scan() {

			if len(scanner.Text()) == 0 {
				continue
			}
			if strings.HasPrefix(scanner.Text(), "BANNER") {
				continue
			}
			if strings.Contains(scanner.Text(), ":telnet") {
				conf := strings.Split(scanner.Text(), ":")

				if len(conf) < 4 {
					continue
				}
				if conf[1] != "telnet" {
					continue
				}
				port, _ := strconv.Atoi(conf[0])
				devPath = conf[3]
				baud := 115200

				var opts []string
				if len(conf) > 4 {
					opts = strings.Split(conf[4], " ")
				}
				if len(opts) > 0 {
					baud, _ = strconv.Atoi(opts[0])
				}
				fmt.Printf("telnet on port %d baud %d, device %s\n", port, baud, devPath)
				w, _ := ser2net.NewSerialWorker(devPath, baud)
				go w.Worker()

				go func() {
					defer wg.Done()

					err := telnet.ListenAndServe(fmt.Sprintf("%s:%d", bindHostname, port), w)
					if nil != err {
						panic(err)
					}
				}()
				wg.Add(1)
			}
		}

		if err := scanner.Err(); err != nil {
			log.Fatal(err)
		}
		wg.Wait()

	} else {
		w, _ := ser2net.NewSerialWorker(devPath, 0)
		go w.Worker()

		err := telnet.ListenAndServe(fmt.Sprintf("%s:%d", bindHostname, port), w)
		if nil != err {
			panic(err)
		}
	}
}
