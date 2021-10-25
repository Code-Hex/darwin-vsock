package vsock

import (
	"context"
	"net"
)

type Listener struct {
	fd    *netFD
	laddr *Addr
}

var _ net.Listener = (*Listener)(nil)

func (l *Listener) Accept() (net.Conn, error) {
	fd, err := l.fd.accept()
	if err != nil {
		return nil, err
	}
	return newConn(fd), nil
}

func (l *Listener) Close() error {
	return l.fd.Close()
}

func (l *Listener) Addr() net.Addr {
	return l.laddr
}

func Listen(network string, laddr *Addr) (*Listener, error) {
	if network != "vsock" {
		return nil, &net.OpError{
			Op:     "listen",
			Net:    network,
			Source: nil,
			Addr:   laddr,
			Err:    net.UnknownNetworkError(network),
		}
	}
	fd, err := socket(context.Background(), laddr, nil)
	if err != nil {
		return nil, err
	}
	return &Listener{
		fd:    fd,
		laddr: laddr,
	}, nil
}
