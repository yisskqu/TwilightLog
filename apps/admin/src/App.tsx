import { BrowserRouter, Navigate, Outlet, Route, Routes } from "react-router-dom";
import { AuthProvider, homePathFor, useAuth } from "./auth";
import type { Role } from "./api";
import { AppShell, LoadingScreen } from "./components/AppShell";
import { AdminHome } from "./pages/AdminHome";
import { AuditPage } from "./pages/AuditPage";
import { EditorHome } from "./pages/EditorHome";
import { ForbiddenPage } from "./pages/ForbiddenPage";
import { LoginPage } from "./pages/LoginPage";
import { UsersPage } from "./pages/UsersPage";

function ProtectedLayout({ roles }: { roles: Role[] }) {
  const { user, loading } = useAuth();
  if (loading) {
    return <LoadingScreen />;
  }
  if (!user) {
    return <Navigate to="/login" replace />;
  }
  if (!roles.includes(user.role)) {
    return <Navigate to="/forbidden" replace />;
  }
  return (
    <AppShell>
      <Outlet />
    </AppShell>
  );
}

function ForbiddenRoute() {
  const { user, loading } = useAuth();
  if (loading) {
    return <LoadingScreen />;
  }
  if (!user) {
    return <Navigate to="/login" replace />;
  }
  return (
    <AppShell>
      <ForbiddenPage />
    </AppShell>
  );
}

function HomeRedirect() {
  const { user, loading } = useAuth();
  if (loading) {
    return <LoadingScreen />;
  }
  if (!user) {
    return <Navigate to="/login" replace />;
  }
  return <Navigate to={homePathFor(user.role)} replace />;
}

export default function App() {
  return (
    <BrowserRouter>
      <AuthProvider>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/forbidden" element={<ForbiddenRoute />} />
          <Route path="/admin" element={<ProtectedLayout roles={["super_admin", "admin"]} />}>
            <Route index element={<AdminHome />} />
            <Route path="users" element={<UsersPage />} />
            <Route path="audit" element={<AuditPage />} />
          </Route>
          <Route path="/editor" element={<ProtectedLayout roles={["editor"]} />}>
            <Route index element={<EditorHome />} />
          </Route>
          <Route path="/" element={<HomeRedirect />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </AuthProvider>
    </BrowserRouter>
  );
}
