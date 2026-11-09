import { useEffect } from "react";
import { BrowserRouter, Route, Routes, useLocation } from "react-router-dom";

import EmptyState from "../components/common/EmptyState";
import BottomNav from "../components/layout/BottomNav";
import Footer from "../components/layout/Footer";
import Header from "../components/layout/Header";
import { SessionProvider } from "../context/SessionContext";
import { ToastProvider } from "../context/ToastContext";
import NotFound from "../pages/NotFound";

// Las rutas se suman con cada parte del frontend:
//   G18  /, /buscar, /cocinas/:id, /login, /registro y el selector de sede
//   V16  /cocina/inventario · V17 /cocina/menu y /cocina/platos
//   L17  /carrito · L18 /pedidos y /cocina (tablero)
//   G21  /admin/cocinas y /admin/metricas
// Las de la cocina y el admin van dentro de <ProtectedRoute> y <WorkspaceLayout>.

function ScrollToTop() {
  const { pathname } = useLocation();

  useEffect(() => {
    window.scrollTo({ top: 0, left: 0, behavior: "auto" });
  }, [pathname]);

  return null;
}

// Pantalla de inicio provisoria hasta que llegue la de la sede elegida (G18).
function InicioProvisorio() {
  return (
    <main className="container page-space">
      <EmptyState title="GhostKitchen" text="Varias marcas, una misma cocina. Las pantallas se van sumando en cada parte del proyecto." />
    </main>
  );
}

function PageFooter() {
  const { pathname } = useLocation();
  return pathname === "/login" ? null : <Footer />;
}

function AppRouter() {
  return (
    <BrowserRouter>
      <ToastProvider>
        <SessionProvider>
          <ScrollToTop />
          <div className="app">
            <Header />
            <Routes>
              <Route path="/" element={<InicioProvisorio />} />
              <Route path="*" element={<NotFound />} />
            </Routes>
            <PageFooter />
            <BottomNav />
          </div>
        </SessionProvider>
      </ToastProvider>
    </BrowserRouter>
  );
}

export default AppRouter;
