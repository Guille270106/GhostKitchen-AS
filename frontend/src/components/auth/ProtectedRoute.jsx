import { Navigate, useLocation } from "react-router-dom";

import { useSession } from "../../context/SessionContext";
import { homeForRole } from "../../utils/auth";

function ProtectedRoute({ allowedRoles, children }) {
  const location = useLocation();
  const { user } = useSession();

  if (!user) {
    return <Navigate to="/login" replace state={{ from: location }} />;
  }

  if (allowedRoles?.length && !allowedRoles.includes(user.rol)) {
    return <Navigate to={homeForRole(user.rol)} replace state={{ permissionError: true }} />;
  }

  return children;
}

export default ProtectedRoute;
