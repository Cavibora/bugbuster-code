//go:build darwin || linux

package main

import (
	"golang.org/x/sys/unix"
)

func dupFd(oldfd int, newfd int) error {
	return unix.Dup2(oldfd, newfd)
}

func dupFdInt(fd int) (int, error) {
	return unix.Dup(fd)
}