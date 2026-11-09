import { useEffect } from "react";
import { BrowserRouter, Route, Routes, useLocation } from "react-router-dom";

import ProtectedRoute from "../components/auth/ProtectedRoute";
import WorkspaceLayout from "../components/layout/WorkspaceLayout";
import StockIngredientes from "../pages/StockIngredientes";
import BottomNav from "../components/layout/BottomNav";
import Footer from "../components/layout/Footer";
import Header from "../components/layout/Header";
import SedeSelector from "../components/sede/SedeSelector";
import { CartProvider } from "../context/CartContext";
import { SessionProvider, useSession } from "../context/SessionContext";
import { ToastProvider } from "../context/ToastContext";
import Cart from "../pages/Cart";
import Home from "../pages/Home";
import KitchenOrders from "../pages/KitchenOrders";
import KitchenDetail from "../pages/KitchenDetail";
import Login from "../pages/Login";
import MyOrders from "../pages/MyOrders";
import NotFound from "../pages/NotFound";
import RecetasMermas from "../pages/RecetasMermas";
import Register from "../pages/Register";
import Search from "../pages/Search";

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

// El selector de sede aparece sobre las pantallas del cliente, no en los paneles ni en el login.
function SedeGate() {
  const { pathname } = useLocation();
  const { user, selectorSedeAbierto } = useSession();
  const panel = /^\/(cocina|admin|login|registro)(\/|$)/.test(pathname) || (user && user.rol !== "CLIENTE");
  return selectorSedeAbierto && !panel ? <SedeSelector /> : null;
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
          <CartProvider>
            <ScrollToTop />
            <div className="app">
              <Header />
              <Routes>
                {/* Públicas: se exploran los restaurantes de la sede elegida */}
                <Route path="/" element={<Home />} />
                <Route path="/buscar" element={<Search />} />
                <Route path="/cocinas/:id" element={<KitchenDetail />} />
                <Route path="/login" element={<Login />} />
                <Route path="/registro" element={<Register />} />

                {/* Cliente */}
                <Route
                  path="/carrito"
                  element={
                    <ProtectedRoute allowedRoles={["CLIENTE"]}>
                      <Cart />
                    </ProtectedRoute>
                  }
                />
                <Route
                  path="/pedidos"
                  element={
                    <ProtectedRoute allowedRoles={["CLIENTE"]}>
                      <MyOrders />
                    </ProtectedRoute>
                  }
                />

                <Route path="/cocina" element={<ProtectedRoute allowedRoles={["COCINA"]}><WorkspaceLayout /></ProtectedRoute>}>
                  <Route index element={<KitchenOrders />} />
                  <Route path="inventario" element={<StockIngredientes />} />
                  <Route path="platos" element={<RecetasMermas />} />
                </Route>
                <Route path="*" element={<NotFound />} />
              </Routes>
              <SedeGate />
              <PageFooter />
              <BottomNav />
            </div>
          </CartProvider>
        </SessionProvider>
      </ToastProvider>
    </BrowserRouter>
  );
}

export default AppRouter;
