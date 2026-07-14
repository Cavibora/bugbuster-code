//go:build windows

package main

import (
	"syscall"
)

func dupFd(oldfd int, newfd int) error {
	return syscall.Dup2(oldfd, newfd)
}

func dupFdInt(fd int) (int, error) {
	return syscall.Dup(fd)
}