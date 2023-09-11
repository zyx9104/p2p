package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	log "github.com/sirupsen/logrus"

	"p2p"
)

var (
	debug    bool
	server   bool
	clientId string
	serverIp string
)

func init() {
	flag.BoolVar(&debug, "d", false, "debug")
	flag.BoolVar(&server, "s", false, "server")
	flag.StringVar(&clientId, "c", "", "client id")
	flag.StringVar(&serverIp, "a", "123.56.16.26:9104", "client id")
	flag.Parse()
	initLog()
}

type MyFormatter struct{}

func (m *MyFormatter) Format(entry *log.Entry) ([]byte, error) {
	var b *bytes.Buffer
	if entry.Buffer != nil {
		b = entry.Buffer
	} else {
		b = &bytes.Buffer{}
	}

	timestamp := entry.Time.Format("2006-01-02 15:04:05")
	var newLog string

	//HasCaller()为true才会有调用信息
	if entry.HasCaller() {
		fName := filepath.Base(entry.Caller.File)
		newLog = fmt.Sprintf("[%s] [%s] [%s:%d %s] %s\n",
			timestamp, entry.Level, fName, entry.Caller.Line, entry.Caller.Function, entry.Message)
	} else {
		newLog = fmt.Sprintf("[%s] [%s] %s\n", timestamp, entry.Level, entry.Message)
	}

	b.WriteString(newLog)
	return b.Bytes(), nil
}

func initLog() {
	log.SetReportCaller(true)
	log.SetFormatter(&MyFormatter{})
	if debug {
		log.SetLevel(log.DebugLevel)
	}
}

func main() {
	log.Infof("is server: %v", server)
	if server {
		s := p2p.NewServer()
		s.Listen()
	} else {
		c := p2p.NewClient(clientId, serverIp)
		for {
			input := bufio.NewScanner(os.Stdin)
			fmt.Print("$: >\n")
			for input.Scan() {
				cmd := input.Text()
				log.Debug(cmd)
				ss := strings.Split(cmd, " ")
				if ss[0] == "conn" {
					log.Debug("conn")
					c.Conn(ss[1])
				} else if ss[0] == "send" {
					log.Debug("send")
					c.SendTo(ss[1], ss[2])
				}
			}
		}
	}
}
