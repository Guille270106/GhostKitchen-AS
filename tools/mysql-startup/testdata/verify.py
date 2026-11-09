"""Verificación descartable de V2–V4. Requiere Docker y Python estándar.

Ejecutar desde cualquier directorio: python tools/mysql-startup/testdata/verify.py
No usa las contraseñas ni los volúmenes del entorno de desarrollo.
"""
import concurrent.futures
import json
from pathlib import Path
import subprocess
import time
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parents[3]
COMPOSE = ["docker", "compose", "-p", "gk-ronda1-check", "-f", str(Path(__file__).with_name("compose.yml"))]


def docker(*args, check=True):
    result = subprocess.run(COMPOSE + list(args), cwd=ROOT, text=True, capture_output=True)
    if check and result.returncode:
        raise RuntimeError(result.stdout + result.stderr)
    return result


def sql(user, password, database, query, check=True):
    return docker("exec", "-T", "-e", "MYSQL_PWD=" + password, "mysql", "mysql",
                  "-N", "-B", "-u" + user, database, "-e", query, check=check)


def request(base, key=None):
    headers = {} if key is None else {"X-API-Key": key}
    req = urllib.request.Request(base + "/public/v1/sedes", headers=headers)
    try:
        with urllib.request.urlopen(req, timeout=10) as response:
            return response.status, dict(response.headers), response.read()
    except urllib.error.HTTPError as error:
        return error.code, dict(error.headers), error.read()


def verificar_gateway():
    # Reconfigurar primero: un puerto efímero puede cambiar al reiniciar el contenedor.
    docker("restart", "gateway")
    port = docker("port", "gateway", "8080").stdout.strip().rsplit(":", 1)[1]
    base = "http://127.0.0.1:" + port
    deadline = time.monotonic() + 10
    while True:
        try:
            with urllib.request.urlopen(base + "/health", timeout=1):
                break
        except (urllib.error.URLError, ConnectionError):
            if time.monotonic() >= deadline:
                raise
            time.sleep(0.1)
    with concurrent.futures.ThreadPoolExecutor(max_workers=40) as pool:
        responses = list(pool.map(lambda _: request(base), range(90)))
    counts = {code: sum(r[0] == code for r in responses) for code in {r[0] for r in responses}}
    assert set(counts) == {401, 429}, counts
    for code, headers, body in responses:
        if code == 429:
            assert headers.get("Retry-After") == "1", headers
            assert json.loads(body)["code"] == "LIMITE_EXCEDIDO", body
    assert request(base, "test-key")[0] == 200, "el cupo por IP afectó a una clave"
    assert request(base, "invalid-key")[0] == 401, "se aceptó una clave inválida"
    with concurrent.futures.ThreadPoolExecutor(max_workers=40) as pool:
        keyed = list(pool.map(lambda _: request(base, "test-key"), range(90)))
    assert {r[0] for r in keyed} == {200, 429}, "se perdió el límite por clave"
    print("OK gateway: sin clave 401/429 por IP; con clave cupo separado y 200/429", flush=True)


def main():
    try:
        print("Iniciando entorno descartable de ronda 1...", flush=True)
        docker("up", "-d", "--wait", "mysql", "busqueda", "gateway")
        print("OK inicialización MySQL y sintaxis real de NGINX", flush=True)
        docker("build", "usuarios", "integration")
        users = [("usuarios", "test-usuarios"), ("inventario", "test'quote\\backslash"), ("pedidos", "test-pedidos")]
        for _ in range(2):
            for user, _ in users:
                docker("run", "--rm", user)
        for user, password in users:
            result = sql(user, password, user + "_db", "SELECT nombre FROM servicio_metadata; SELECT version, dirty FROM schema_migrations;")
            version = "4" if user == "inventario" else "1"
            assert result.stdout.splitlines() == [user, version + "\t0"], result.stdout
            for other, _ in users:
                if user == other:
                    continue
                denied = sql(user, password, user + "_db", "SELECT * FROM " + other + "_db.servicio_metadata;", check=False)
                assert denied.returncode and "denied" in denied.stderr.lower(), denied.stdout + denied.stderr
        print("OK tres migraciones, repetición y seis accesos cruzados denegados", flush=True)
        docker("restart", "mysql")
        docker("up", "-d", "--wait", "mysql")
        assert "1\t0" in sql("usuarios", "test-usuarios", "usuarios_db", "SELECT version, dirty FROM schema_migrations;").stdout
        print("OK persistencia tras reiniciar MySQL", flush=True)
        integration = docker("run", "--rm", "integration")
        print(integration.stdout, flush=True)
        started = docker("run", "--rm", "usuarios", "/bin/echo", "SERVICE_STARTED")
        assert "SERVICE_STARTED" in started.stdout
        blocked = docker("run", "--rm", "-e", "DB_USER=prueba", "-e", "DB_PASSWORD=test-prueba",
                         "-e", "DB_NAME=integration_db", "usuarios", "/bin/echo", "MUST_NOT_START", check=False)
        assert blocked.returncode != 0 and "MUST_NOT_START" not in blocked.stdout, blocked.stdout
        print("OK el comando arranca solo con migraciones exitosas", flush=True)
        verificar_gateway()
        print("OK ronda 1: verificaciones completas", flush=True)
    finally:
        # Solo elimina los recursos del proyecto de prueba y su volumen descartable.
        cleanup = docker("down", "-v", "--remove-orphans", check=False)
        if cleanup.returncode:
            print("Revisar limpieza del entorno de prueba:", cleanup.stderr, flush=True)


if __name__ == "__main__":
    main()
