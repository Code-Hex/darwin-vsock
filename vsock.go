package vsock

import (
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

// xnu example code
// https://github.com/apple/darwin-xnu/blob/8f02f2a044b9bb1ad951987ef5bab20ec9486310/tests/vsock.c

type Addr struct {
	CID  uint32
	Port uint32
}

var _ net.Addr = (*Addr)(nil)

func (a *Addr) Network() string { return "vsock" }
func (a *Addr) String() string  { return fmt.Sprintf("%d:%d", a.CID, a.Port) }

func (a *Addr) family() int      { return unix.AF_VSOCK }
func (a *Addr) isWildcard() bool { return a.CID == unix.VMADDR_CID_ANY }
func (a *Addr) sockaddr(family int) (unix.Sockaddr, error) {
	if a == nil {
		return nil, nil
	}
	return &unix.SockaddrVM{
		CID:  a.CID,
		Port: a.Port,
	}, nil
}

func GetLocalCID(fd int) (uint, error) {
	// https://github.com/apple/darwin-xnu/blob/8f02f2a044b9bb1ad951987ef5bab20ec9486310/tests/vsock.c#L58
	intCID, err := unix.IoctlGetInt(fd, unix.IOCTL_VM_SOCKETS_GET_LOCAL_CID)
	return uint(intCID), err
}
