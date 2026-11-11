// Recorrido principal en el navegador: un cliente nuevo busca un plato, lo agrega al carrito y
// confirma el pedido con otra dirección de entrega; la cocina lo ve en su tablero y lo acepta, y
// el cliente ve el cambio en "Mis pedidos". Todo contra el sistema real (frontend, gateway y
// servicios): no hay respuestas simuladas.
//
// Los botones y campos se buscan por lo que ve el usuario (su texto o su rótulo), no por clases
// de CSS: si cambia el diseño la prueba sigue andando; si cambia lo que dice la pantalla, hay
// que actualizarla.
import { expect, test } from "@playwright/test";

import { prepararDatos } from "./preparar.js";

const SEDE = "Cocina Cerro";
const DIRECCION_DEL_PEDIDO = "Av. Colón 1234, 3° B";

let datos;
test.beforeAll(async () => {
  datos = await prepararDatos();
});

// pantalla es el navegador del cliente; la cocina usa otro navegador, con su propia sesión.
test("el cliente pide un plato y la cocina lo acepta", async ({ page: pantalla, browser }) => {
  const { plato, cliente, cocina } = datos;
  const navegadorCocina = await browser.newContext();
  let codigo;

  await test.step("el cliente se registra", async () => {
    await pantalla.goto("/registro");
    await pantalla.getByLabel("Nombre").fill(cliente.nombre);
    await pantalla.getByLabel("Email").fill(cliente.email);
    await pantalla.getByPlaceholder("Mínimo 8 caracteres").fill(cliente.password);
    await pantalla.getByLabel("Dirección").fill(cliente.direccion);
    await pantalla.getByRole("button", { name: "Crear cuenta" }).click();
    await expect(pantalla.getByText("¡Bienvenido/a!")).toBeVisible();
  });

  await test.step("elige la sede", async () => {
    const selector = pantalla.getByRole("dialog", { name: "¿Desde qué cocina te mandamos tu pedido?" });
    await selector.getByRole("button", { name: SEDE }).click();
    await expect(selector).toBeHidden();
  });

  await test.step("busca el plato", async () => {
    await pantalla.getByRole("combobox", { name: "Buscar comida" }).fill(plato.nombre);
    await pantalla.getByRole("button", { name: "Buscar", exact: true }).click();
    await expect(pantalla).toHaveURL(/\/buscar\?q=/);
    // Búsqueda se entera del plato nuevo y de su stock por mensajes (consistencia eventual): se
    // recarga hasta que aparece con el botón habilitado.
    const agregar = pantalla.getByRole("button", { name: `Agregar ${plato.nombre}` });
    await expect(async () => {
      await pantalla.reload();
      await expect(agregar).toBeEnabled({ timeout: 3000 });
    }).toPass({ timeout: 45_000, intervals: [2000] });
  });

  await test.step("lo agrega al carrito", async () => {
    await pantalla.getByRole("button", { name: `Agregar ${plato.nombre}` }).click();
    await expect(pantalla.getByText(`Agregaste ${plato.nombre}`)).toBeVisible();
    await pantalla.getByRole("link", { name: "Carrito con 1 productos" }).click();
    await expect(pantalla.getByRole("heading", { name: "Tu pedido" })).toBeVisible();
    await expect(pantalla.getByRole("heading", { name: plato.nombre })).toBeVisible();
  });

  await test.step("confirma con otra dirección de entrega", async () => {
    const direccion = pantalla.getByRole("textbox", { name: /Entregar en/ });
    // Propone la dirección de la cuenta; para este pedido la cambia.
    await expect(direccion).toHaveValue(cliente.direccion);
    await direccion.fill(DIRECCION_DEL_PEDIDO);
    await pantalla.getByRole("button", { name: "Confirmar pedido" }).click();
    await expect(pantalla).toHaveURL(/\/pedidos$/);
  });

  await test.step("lo ve en Mis pedidos", async () => {
    const tarjeta = pantalla.getByRole("article").filter({ hasText: plato.nombre });
    await expect(tarjeta.getByRole("heading", { name: "Recibimos tu pedido" })).toBeVisible();
    await expect(tarjeta).toContainText(`Entregar en ${DIRECCION_DEL_PEDIDO}`);
    codigo = (await tarjeta.textContent()).match(/Pedido #(\w+)/)[1];
  });

  await test.step("la cocina lo ve en su tablero y lo acepta", async () => {
    const tablero = await navegadorCocina.newPage();
    await tablero.goto("/login");
    await tablero.getByLabel("Email").fill(cocina.email);
    await tablero.getByPlaceholder("••••••••").fill(cocina.password);
    await tablero.getByRole("button", { name: "Ingresar" }).click();
    await expect(tablero.getByRole("heading", { name: "Pedidos de hoy" })).toBeVisible();

    const comanda = tablero.getByRole("article").filter({ hasText: `Pedido #${codigo}` });
    await expect(comanda).toContainText(DIRECCION_DEL_PEDIDO);
    await expect(comanda).toContainText(plato.nombre);
    await comanda.getByRole("button", { name: "Aceptar pedido" }).click();
    await expect(tablero.getByText(`Aceptaste el pedido #${codigo}`)).toBeVisible();
    await expect(comanda.getByRole("button", { name: "Empezar a preparar" })).toBeVisible();
  });

  await test.step("el cliente ve que la cocina lo aceptó", async () => {
    await pantalla.reload();
    const tarjeta = pantalla.getByRole("article").filter({ hasText: plato.nombre });
    await expect(tarjeta.getByRole("heading", { name: "La cocina aceptó tu pedido" })).toBeVisible();
    // Aceptado ya no se puede cancelar.
    await expect(tarjeta.getByRole("button", { name: "Cancelar pedido" })).toBeHidden();
  });

  await navegadorCocina.close();
});
