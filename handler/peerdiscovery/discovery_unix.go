//go:build !windows

package peerdiscovery

import "syscall"

// setSocketOptions sets reuse options so multiple sockets can bind the same port.
func setSocketOptions(network, address string, c syscall.RawConn) error {
	var soErr error
	err := c.Control(func(fd uintptr) {
		// Allow reuse of address
		soErr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
		if soErr != nil {
			return
		}
		// Note: SO_REUSEPORT omitted for portability; SO_REUSEADDR is set.
	})
	if err != nil {
		return err
	}
	return soErr
}
