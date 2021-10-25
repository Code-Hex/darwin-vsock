package vsock

import (
	"context"
	"net"
)

func Dial(network string, laddr, raddr *Addr) (net.Conn, error) {
	return DialContext(context.Background(), network, laddr, raddr)
}

func DialContext(ctx context.Context, network string, laddr, raddr *Addr) (net.Conn, error) {
	if network != "vsock" {
		return nil, &net.OpError{
			Op:     "dial",
			Net:    network,
			Source: laddr,
			Addr:   raddr,
			Err:    net.UnknownNetworkError(network),
		}
	}
	fd, err := socket(ctx, laddr, raddr)
	if err != nil {
		return nil, err
	}
	return newConn(fd), nil
}
