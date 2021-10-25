package vsock

// https://github.com/golang/go/blob/go1.17.2/src/net/fd_posix.go

import (
	"context"
	"net"
	"os"
	"time"
	_ "unsafe"

	"golang.org/x/sys/unix"
)

// Network file descriptor.
//
// https://github.com/golang/go/blob/8dbf3e9393400d72d313e5616c88873e07692c70/src/net/fd_posix.go#L18
type netFD struct {
	fd   int
	file *os.File

	// immutable until Close
	family int
	sotype int
	net    string
	laddr  net.Addr // local
	raddr  net.Addr // remote
}

// Close will be called when caused something error in socket.
func (fd *netFD) Close() error { return fd.file.Close() }

func (fd *netFD) setAddr(laddr, raddr net.Addr) {
	fd.laddr = laddr
	fd.raddr = raddr
}

// func (fd *netFD) name() string {
// 	var ls, rs string
// 	if fd.laddr != nil {
// 		ls = fd.laddr.String()
// 	}
// 	if fd.raddr != nil {
// 		rs = fd.raddr.String()
// 	}
// 	return fd.net + ":" + ls + "->" + rs
// }

func (fd *netFD) accept() (*netFD, error) {
	nfd, rsa, err := unix.Accept(fd.fd)
	if err != nil {
		return nil, err
	}
	acceptFd := newFD(nfd, "accept")
	lsa, _ := unix.Getsockname(acceptFd.fd)
	fd.laddr = sockaddrToVSock(lsa)
	fd.raddr = sockaddrToVSock(rsa)
	return acceptFd, nil
}

//go:linkname setDefaultListenerSockopts net.setDefaultListenerSockopts
func setDefaultListenerSockopts(s int) error

//go:linkname listenerBacklog net.listenerBacklog
func listenerBacklog() int

//go:linkname sysSocket net.sysSocket
func sysSocket(family, sotype, proto int) (int, error)

func socket(ctx context.Context, laddr, raddr *Addr) (*netFD, error) {
	// https://github.com/apple/darwin-xnu/blob/8f02f2a044b9bb1ad951987ef5bab20ec9486310/tests/vsock.c#L49
	// https://github.com/golang/go/blob/f448cb8ba83be1055cc73101e0c217c2a503c8ad/src/net/sys_cloexec.go#L21
	socketFd, err := sysSocket(unix.AF_VSOCK, unix.SOCK_STREAM, 0)
	if err != nil {
		return nil, err
	}

	fd := newFD(socketFd, "socket")
	fd.setAddr(laddr, raddr)

	if laddr != nil && raddr == nil {
		if err := fd.listenStream(laddr, listenerBacklog()); err != nil {
			fd.Close()
			return nil, err
		}
		return fd, nil
	}
	if err := fd.dial(ctx, laddr, raddr); err != nil {
		fd.Close()
		return nil, err
	}
	return fd, nil
}

func (fd *netFD) listenStream(laddr *Addr, backlog int) error {
	if err := setDefaultListenerSockopts(fd.fd); err != nil {
		return err
	}
	lsa, err := laddr.sockaddr(unix.AF_VSOCK)
	if err != nil {
		return err
	}
	if err := unix.Bind(fd.fd, lsa); err != nil {
		return os.NewSyscallError("bind", err)
	}
	if err = unix.Listen(fd.fd, backlog); err != nil {
		return os.NewSyscallError("listen", err)
	}
	return nil
}

// https://github.com/golang/go/blob/go1.17.2/src/net/sock_posix.go#L117
func (fd *netFD) dial(ctx context.Context, laddr, raddr *Addr) error {
	var (
		lsa  unix.Sockaddr
		rsa  unix.Sockaddr // remote address from the user
		crsa unix.Sockaddr // remote address we actually connected to
	)
	if laddr != nil {
		lsa, _ := laddr.sockaddr(unix.AF_VSOCK)
		if err := unix.Bind(fd.fd, lsa); err != nil {
			return os.NewSyscallError("bind", err)
		}
	}
	if raddr != nil {
		rsa, _ := raddr.sockaddr(unix.AF_VSOCK)
		var err error
		if crsa, err = fd.connect(ctx, lsa, rsa); err != nil {
			return err
		}
	}

	lsa, _ = unix.Getsockname(fd.fd)
	if crsa != nil {
		fd.laddr = sockaddrToVSock(lsa)
		fd.raddr = sockaddrToVSock(crsa)
	} else if rsa, _ = unix.Getpeername(fd.fd); rsa != nil {
		fd.laddr = sockaddrToVSock(lsa)
		fd.raddr = sockaddrToVSock(rsa)
	} else {
		fd.laddr = sockaddrToVSock(lsa)
		fd.raddr = raddr
	}
	return nil
}

var (
	noDeadline   = time.Time{}
	aLongTimeAgo = time.Unix(1, 0)
)

// https://github.com/golang/go/blob/go1.17.2/src/net/fd_unix.go#L56
func (fd *netFD) connect(ctx context.Context, lsa, rsa unix.Sockaddr) (retrsa unix.Sockaddr, retErr error) {
	switch err := unix.Connect(fd.fd, rsa); err {
	case nil, unix.EISCONN:
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		return nil, nil
	case unix.EINPROGRESS, unix.EALREADY, unix.EINTR:
	default:
		return nil, os.NewSyscallError("connect", err)
	}

	if ctx != context.Background() {
		if deadline, ok := ctx.Deadline(); ok {
			fd.file.SetWriteDeadline(deadline)
			defer fd.file.SetWriteDeadline(noDeadline)
		}

		// Wait for the interrupter goroutine to exit before returning
		// from connect.
		done := make(chan struct{})
		interruptRes := make(chan error)
		defer func() {
			close(done)
			if ctxErr := <-interruptRes; ctxErr != nil && retErr == nil {
				retErr = ctxErr
				fd.file.Close() // prevent a leak
			}
		}()
		go func() {
			select {
			case <-ctx.Done():
				// Force the runtime's poller to immediately give up
				// waiting for writability, unblocking waitWrite
				// below.
				fd.file.SetWriteDeadline(aLongTimeAgo)
				interruptRes <- ctx.Err()
			case <-done:
				interruptRes <- nil
			}
		}()
	}

	for {
		nerr, err := unix.GetsockoptInt(fd.fd, unix.SOL_SOCKET, unix.SO_ERROR)
		if err != nil {
			return nil, os.NewSyscallError("getsockopt", err)
		}

		switch err := unix.Errno(nerr); err {
		case unix.EINPROGRESS, unix.EALREADY, unix.EINTR:
		case unix.EISCONN:
			return nil, nil
		case unix.Errno(0):
			// The runtime poller can wake us up spuriously;
			// see issues 14548 and 19289. Check that we are
			// really connected; if not, wait again.
			if rsa, err := unix.Getpeername(fd.fd); err == nil {
				return rsa, nil
			}
		default:
			return nil, os.NewSyscallError("connect", err)
		}
	}
}

func newFD(sysfd int, name string) *netFD {
	return &netFD{
		fd:     sysfd,
		file:   os.NewFile(uintptr(sysfd), name),
		family: unix.AF_UNIX,
		sotype: unix.SOCK_STREAM,
		net:    "vsock",
	}
}

func sockaddrToVSock(sa unix.Sockaddr) net.Addr {
	if s, ok := sa.(*unix.SockaddrVM); ok {
		return &Addr{
			CID:  s.CID,
			Port: s.Port,
		}
	}
	return nil
}
