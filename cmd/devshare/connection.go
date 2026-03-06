package main

import (
	"github.com/gorilla/websocket"
	"io"
	"net"
	"sync"
	"time"
)

type tunnelConn struct {
	*websocket.Conn
	r  io.Reader
	mu sync.Mutex
}

func (c *tunnelConn) Read(p []byte) (int, error) {
	for {
		if c.r != nil {
			n, e := c.r.Read(p)
			if e == io.EOF {
				c.r = nil
				if n > 0 {
					return n, nil
				}
				continue
			}
			return n, e
		}
		mt, r, e := c.NextReader()
		if e != nil {
			return 0, e
		}
		if mt == websocket.BinaryMessage {
			c.r = r
		}
	}
}

func (c *tunnelConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	e := c.WriteMessage(websocket.BinaryMessage, p)
	if e != nil {
		return 0, e
	}
	return len(p), nil
}

func (c *tunnelConn) LocalAddr() net.Addr { return dummyAddr("local") }

func (c *tunnelConn) RemoteAddr() net.Addr { return dummyAddr("remote") }

func (c *tunnelConn) SetDeadline(t time.Time) error { return nil }

func (c *tunnelConn) SetReadDeadline(t time.Time) error { return nil }

func (c *tunnelConn) SetWriteDeadline(t time.Time) error { return nil }

type dummyAddr string

func (d dummyAddr) Network() string { return "websocket" }

func (d dummyAddr) String() string { return string(d) }
