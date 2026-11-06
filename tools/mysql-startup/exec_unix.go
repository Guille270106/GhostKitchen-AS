//go:build !windows

package main

import (
	"os"
	"os/exec"
	"syscall"
)

// Reemplazar el proceso mantiene SIGTERM y el código de salida del servicio.
func ejecutar(args []string) error {
	path, err := exec.LookPath(args[0])
	if err != nil {
		return err
	}
	return syscall.Exec(path, args, os.Environ())
}
