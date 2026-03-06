package main

import (
	"fmt"
	"github.com/gorilla/websocket"
	"github.com/hashicorp/yamux"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

func serve() {
	fs, pub, keep, ttl := parseShareFlags("serve", os.Args[2:])
	_ = fs.Parse(os.Args[2:])
	if fs.NArg() != 1 {
		log.Fatal("usage: devshare serve <port> [--public] [--ttl 2h]")
	}
	port, e := strconv.Atoi(fs.Arg(0))
	if e != nil || port < 1 || port > 65535 {
		log.Fatal("invalid port")
	}
	c := client()
	out := createRemote(c, "tunnel", *pub, *keep, *ttl)
	fmt.Println(out.URL)
	wsURL := strings.Replace(c.URL, "https://", "wss://", 1)
	wsURL = strings.Replace(wsURL, "http://", "ws://", 1) + "/v1/tunnels/" + out.ID + "/connect"
	headers := http.Header{"Authorization": []string{"Bearer " + out.TunnelSecret}}
	for {
		ws, _, e := websocket.DefaultDialer.Dial(wsURL, headers)
		if e != nil {
			log.Printf("connect: %v; retrying", e)
			time.Sleep(2 * time.Second)
			continue
		}
		session, e := yamux.Client(&tunnelConn{Conn: ws}, nil)
		if e != nil {
			_ = ws.Close()
			continue
		}
		for {
			stream, e := session.Accept()
			if e != nil {
				break
			}
			go func(conn net.Conn) {
				up, e := net.Dial("tcp", fmt.Sprintf("127.0.0.1:%d", port))
				if e != nil {
					_ = conn.Close()
					return
				}
				go func() { _, _ = io.Copy(up, conn); _ = up.(*net.TCPConn).CloseWrite() }()
				_, _ = io.Copy(conn, up)
				_ = conn.Close()
				_ = up.Close()
			}(stream)
		}
		_ = session.Close()
		_ = ws.Close()
		time.Sleep(time.Second)
	}
}
