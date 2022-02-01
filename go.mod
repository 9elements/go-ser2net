module github.com/9elements.com/go-ser2net

go 1.14

require (
	github.com/9elements/go-ser2net v0.0.0-20210518094450-dc0adcca6e31
	github.com/PatrickRudolph/telnet v0.0.0-20210301083732-6a03c1f7971f
	github.com/yudai/gotty v1.0.1
	go.bug.st/serial.v1 v0.0.0-20191202182710-24a6610f0541
)

replace github.com/yudai/gotty v1.0.1 => github.com/yudai/gotty v2.0.0-alpha.3+incompatible

replace github.com/codegangsta/cli v1.22.5 => github.com/urfave/cli v1.22.5
