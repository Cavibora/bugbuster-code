//go:build windows

package main

import (
	"errors"
	"syscall"
)

func stdinSelect(tv *syscall.Timeval, readFds *syscall.FdSet) (bool, error) {
	// Windows doesn't support syscall.Select the same way
	// Fall back to a simple check
	return false, errors.New("stdin select not supported on Windows")
}