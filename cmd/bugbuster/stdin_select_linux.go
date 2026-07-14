//go:build linux

package main

import (
	"syscall"
)

func stdinSelect(tv *syscall.Timeval, readFds *syscall.FdSet) (bool, error) {
	n, err := syscall.Select(1, readFds, nil, nil, tv)
	return n > 0, err
}