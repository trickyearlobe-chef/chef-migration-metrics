import {
  createContext,
  useContext,
  useState,
  useEffect,
  useCallback,
  type ReactNode,
} from "react";
import { login as apiLogin, logout as apiLogout, fetchMe } from "../api";
import { ApiError } from "../api";
import type { MeResponse } from "../types";

// ---------------------------------------------------------------------------
// Auth context — stores the current user's authentication state and provides
// login/logout actions. On mount it calls GET /api/v1/auth/me to restore the
// session from the HTTP-only cookie (if one exists). All child components can
// use `useAuth()` to check auth state and enforce access control.
// ---------------------------------------------------------------------------

export interface AuthUser {
  username: string;
  displayName: string;
  email?: string;
  role: string;
  provider: string;
}

interface AuthContextValue {
  /** The currently authenticated user, or null if not logged in. */
  user: AuthUser | null;
  /** True while the initial session check is in progress. */
  loading: boolean;
  /** Error message from the most recent login attempt. */
  error: string | null;
  /** True if a login request is currently in flight. */
  loggingIn: boolean;
  /** Authenticate with username and password. */
  login: (username: string, password: string) => Promise<boolean>;
  /** Invalidate the current session. */
  logout: () => Promise<void>;
  /** True if the user has the admin role. */
  isAdmin: boolean;
  /** True if the user is authenticated (any role). */
  isAuthenticated: boolean;
}

const AuthContext = createContext<AuthContextValue | undefined>(undefined);

function meToUser(me: MeResponse): AuthUser {
  return {
    username: me.username,
    displayName: me.display_name,
    email: me.email,
    role: me.role,
    provider: me.provider,
  };
}

export function AuthProvider({ children }: { children: ReactNode }) {
  const [user, setUser] = useState<AuthUser | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [loggingIn, setLoggingIn] = useState(false);

  // On mount, try to restore the session from the HTTP-only cookie by
  // calling /api/v1/auth/me. If it fails with 401, the user is simply
  // not logged in.
  useEffect(() => {
    let cancelled = false;

    fetchMe()
      .then((me) => {
        if (!cancelled) {
          setUser(meToUser(me));
        }
      })
      .catch(() => {
        // Not authenticated — that's fine.
        if (!cancelled) {
          setUser(null);
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false);
        }
      });

    return () => {
      cancelled = true;
    };
  }, []);

  const login = useCallback(
    async (username: string, password: string): Promise<boolean> => {
      setError(null);
      setLoggingIn(true);
      try {
        const resp = await apiLogin({ username, password });
        // After login, refresh user info from /me to get full profile.
        // Fall back to the login response user info if /me fails.
        let authUser: AuthUser;
        try {
          const me = await fetchMe();
          authUser = meToUser(me);
        } catch {
          authUser = {
            username: resp.user.username,
            displayName: resp.user.display_name,
            role: resp.user.role,
            provider: "local",
          };
        }
        setUser(authUser);
        return true;
      } catch (err: unknown) {
        const message =
          err instanceof ApiError
            ? err.message
            : err instanceof Error
              ? err.message
              : "Login failed.";
        setError(message);
        return false;
      } finally {
        setLoggingIn(false);
      }
    },
    [],
  );

  const logoutFn = useCallback(async () => {
    try {
      await apiLogout();
    } catch {
      // Even if the API call fails, clear local state.
    }
    setUser(null);
    setError(null);
  }, []);

  const isAuthenticated = user !== null;
  const isAdmin = user?.role === "admin";

  return (
    <AuthContext.Provider
      value={{
        user,
        loading,
        error,
        loggingIn,
        login,
        logout: logoutFn,
        isAdmin,
        isAuthenticated,
      }}
    >
      {children}
    </AuthContext.Provider>
  );
}

/**
 * Hook to access the authentication context.
 * Must be used inside an `<AuthProvider>`.
 */
export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (ctx === undefined) {
    throw new Error("useAuth must be used within an <AuthProvider>");
  }
  return ctx;
}
