//go:build darwin

package main

import (
	"syscall"
)

func stdinSelect(tv *syscall.Timeval, readFds *syscall.FdSet) (bool, error) {
	err := syscall.Select(1, readFds, nil, nil, tv)
	return err == nil, err
}