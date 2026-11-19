import { BrowserRouter, Navigate, Outlet, Route, Routes } from "react-router-dom";
import { AuthProvider, homePathFor, useAuth } from "./auth";
import type { Role } from "./api";
import { AppShell, LoadingScreen } from "./components/AppShell";
import { AdminHome } from "./pages/AdminHome";
import { AuditPage } from "./pages/AuditPage";
import { ContentEditorPage } from "./pages/ContentEditorPage";
import { ForbiddenPage } from "./pages/ForbiddenPage";
import { LoginPage } from "./pages/LoginPage";
import { MediaTestPage } from "./pages/MediaTestPage";
import { NotificationsPage } from "./pages/NotificationsPage";
import { RecommendationsPage } from "./pages/RecommendationsPage";
import { RecycleBinPage } from "./pages/RecycleBinPage";
import { ReviewPage } from "./pages/ReviewPage";
import { SystemPage } from "./pages/SystemPage";
import { TagsPage } from "./pages/TagsPage";
import { UsersPage } from "./pages/UsersPage";
import { WorkspacePage } from "./pages/WorkspacePage";

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

function NotificationRedirect() {
  const { isAdmin } = useAuth();
  return <Navigate to={isAdmin ? "/admin/notifications" : "/editor/notifications"} replace />;
}

export default function App() {
  return (
    <BrowserRouter>
      <AuthProvider>
        <Routes>
          <Route path="/login" element={<LoginPage />} />
          <Route path="/forbidden" element={<ForbiddenRoute />} />
          <Route path="/notifications" element={<NotificationRedirect />} />
          <Route path="/admin" element={<ProtectedLayout roles={["super_admin", "admin"]} />}>
            <Route index element={<AdminHome />} />
            <Route path="workspace/:type" element={<WorkspacePage />} />
            <Route path="content/new" element={<ContentEditorPage />} />
            <Route path="content/:type/:id" element={<ContentEditorPage />} />
            <Route path="reviews" element={<ReviewPage />} />
            <Route path="recommendations" element={<RecommendationsPage />} />
            <Route path="recycle" element={<RecycleBinPage />} />
            <Route path="notifications" element={<NotificationsPage />} />
            <Route path="tags" element={<TagsPage />} />
            <Route path="system" element={<SystemPage />} />
            <Route path="users" element={<UsersPage />} />
            <Route path="audit" element={<AuditPage />} />
            <Route path="media" element={<MediaTestPage />} />
          </Route>
          <Route path="/editor" element={<ProtectedLayout roles={["editor"]} />}>
            <Route index element={<Navigate to="/editor/workspace/article_column" replace />} />
            <Route path="workspace/:type" element={<WorkspacePage />} />
            <Route path="content/new" element={<ContentEditorPage />} />
            <Route path="content/:type/:id" element={<ContentEditorPage />} />
            <Route path="recycle" element={<RecycleBinPage />} />
            <Route path="notifications" element={<NotificationsPage />} />
            <Route path="media" element={<MediaTestPage />} />
          </Route>
          <Route path="/" element={<HomeRedirect />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </AuthProvider>
    </BrowserRouter>
  );
}
