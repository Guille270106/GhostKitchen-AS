package mysql

import (
	"errors"
	"fmt"
	"testing"

	driver "github.com/go-sql-driver/mysql"
)

func TestEsClaveRepetida(t *testing.T) {
	repetida := &driver.MySQLError{Number: 1062, Message: "Duplicate entry"}
	if !esClaveRepetida(repetida) || !esClaveRepetida(fmt.Errorf("insertar: %w", repetida)) {
		t.Fatal("el error 1062 tiene que reconocerse, aunque venga envuelto")
	}
	if esClaveRepetida(&driver.MySQLError{Number: 3819}) || esClaveRepetida(errors.New("otro")) || esClaveRepetida(nil) {
		t.Fatal("solo el 1062 es clave repetida")
	}
}

func TestNulo(t *testing.T) {
	if nulo("") != nil || nulo("cerro") != "cerro" {
		t.Fatal(`"" tiene que guardarse como NULL`)
	}
}
