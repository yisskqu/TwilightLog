import { createContext, useCallback, useContext, useEffect, useMemo, useState, type ReactNode } from "react";
import { fetchMe, loginRequest, logoutRequest, refreshSession, type Role, type User } from "./api";

type AuthContextValue = {
  user: User | null;
  loading: boolean;
  login: (username: string, password: string, remember: boolean) => Promise<User>;
  logout: () => Promise<void>;
  reload: () => Promise<void>;
  isAdmin: boolean;
  isEditor: boolean;
};

const AuthContext = createContext<AuthContextValue | null>(null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<User | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    async function restore() {
      try {
        const restored = await refreshSession();
        if (!cancelled) {
          setUser(restored);
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    }
    void restore();
    return () => {
      cancelled = true;
    };
  }, []);

  const login = useCallback(async (username: string, password: string, remember: boolean) => {
    const loggedIn = await loginRequest(username, password, remember);
    setUser(loggedIn);
    return loggedIn;
  }, []);

  const logout = useCallback(async () => {
    try {
      await logoutRequest();
    } finally {
      setUser(null);
    }
  }, []);

  const reload = useCallback(async () => {
    const current = await fetchMe();
    setUser(current);
  }, []);

  const value = useMemo<AuthContextValue>(() => {
    const role: Role | null = user?.role ?? null;
    return {
      user,
      loading,
      login,
      logout,
      reload,
      isAdmin: role === "super_admin" || role === "admin",
      isEditor: role === "editor",
    };
  }, [user, loading, login, logout, reload]);

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const context = useContext(AuthContext);
  if (!context) {
    throw new Error("useAuth must be used inside AuthProvider");
  }
  return context;
}

export function homePathFor(role: Role): string {
  if (role === "super_admin" || role === "admin") {
    return "/admin";
  }
  if (role === "editor") {
    return "/editor";
  }
  return "/forbidden";
}
