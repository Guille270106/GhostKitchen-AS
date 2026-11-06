package main

import (
	"os"
	"os/exec"
)

// Para uso local; los contenedores ejecutan la variante Unix.
func ejecutar(args []string) error {
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
