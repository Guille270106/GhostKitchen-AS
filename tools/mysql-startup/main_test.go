package main

import (
	"os"
	"sync"
	"testing"
)

func TestConfiguracionIncompletaNoInicia(t *testing.T) {
	for _, key := range []string{"DB_USER", "DB_PASSWORD", "DB_NAME"} {
		t.Setenv(key, "")
	}
	if _, err := cargar(); err == nil {
		t.Fatal("aceptó credenciales incompletas")
	}
}

func TestPasswordNoSeInterpretaComoDSN(t *testing.T) {
	c := config{host: "localhost", port: "3306", user: "inventario", database: "inventario_db",
		password: "p@ss:/?#'\\word"}
	db, err := conectar(c)
	if err != nil {
		t.Fatal("la contraseña no debería requerir escape de URL")
	}
	db.Close()
}

// Ejecutar solo contra una base descartable con MYSQL_STARTUP_INTEGRATION=1.
func TestIntegracionMigracionYFallo(t *testing.T) {
	if os.Getenv("MYSQL_STARTUP_INTEGRATION") != "1" {
		t.Skip("requiere MySQL real y base descartable")
	}
	c, err := cargar()
	if err != nil {
		t.Fatal(err)
	}
	c.directory = t.TempDir()
	escribir := func(name, sql string) {
		t.Helper()
		if err := os.WriteFile(c.directory+"/"+name, []byte(sql), 0600); err != nil {
			t.Fatal(err)
		}
	}
	escribir("000001_prueba.up.sql", "CREATE TABLE prueba_migracion (id INT PRIMARY KEY);")
	escribir("000001_prueba.down.sql", "DROP TABLE prueba_migracion;")
	// Dos arranques simultáneos comparten el bloqueo de migraciones en MySQL.
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); results <- aplicar(c) }()
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatal(err)
		}
	}
	db, err := conectar(c)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var rows int
	if err := db.QueryRow("SELECT COUNT(*) FROM prueba_migracion").Scan(&rows); err != nil {
		t.Fatal("no se creó la tabla de la migración:", err)
	}
	if err := aplicar(c); err != nil {
		t.Fatalf("reinicio no idempotente: %v", err)
	}
	escribir("000002_rota.up.sql", "ESTO NO ES SQL;")
	if err := aplicar(c); err == nil {
		t.Fatal("aceptó SQL inválido")
	}
	// Aunque se corrija el archivo, requiere reparación explícita del estado dirty.
	escribir("000002_rota.up.sql", "SELECT 1;")
	if err := aplicar(c); err == nil {
		t.Fatal("ignoró el estado dirty")
	}
}
