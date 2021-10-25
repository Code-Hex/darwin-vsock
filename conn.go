package vsock

import (
	"net"
	"time"
)

type Conn struct {
	fd *netFD
}

var _ net.Conn = (*Conn)(nil)

func newConn(fd *netFD) *Conn {
	return &Conn{
		fd: fd,
	}
}

func (c *Conn) Read(b []byte) (n int, err error)   { return c.fd.file.Read(b) }
func (c *Conn) Write(b []byte) (n int, err error)  { return c.fd.file.Write(b) }
func (c *Conn) Close() error                       { return c.fd.file.Close() }
func (c *Conn) LocalAddr() net.Addr                { return c.fd.laddr }
func (c *Conn) RemoteAddr() net.Addr               { return c.fd.raddr }
func (c *Conn) SetDeadline(t time.Time) error      { return c.fd.file.SetDeadline(t) }
func (c *Conn) SetReadDeadline(t time.Time) error  { return c.fd.file.SetReadDeadline(t) }
func (c *Conn) SetWriteDeadline(t time.Time) error { return c.fd.file.SetWriteDeadline(t) }
