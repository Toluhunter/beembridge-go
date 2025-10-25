//go:build windows

package peerdiscovery

import "syscall"

func setSocketOptions(network, address string, c syscall.RawConn) error {
	var soErr error
	err := c.Control(func(fd uintptr) {
		soErr = syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
	})
	if err != nil {
		return err
	}
	return soErr
}
