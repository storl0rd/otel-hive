import React, {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useRef,
  useState,
} from "react";

import { authApi } from "@/api/auth";
import type { MeResponse, TokenPair } from "@/api/auth";

// ─── Storage keys ─────────────────────────────────────────────────────────────

const ACCESS_TOKEN_KEY = "otel_hive_access_token";
const REFRESH_TOKEN_KEY = "otel_hive_refresh_token";
const EXPIRES_AT_KEY = "otel_hive_expires_at";

export function getStoredAccessToken(): string | null {
  return localStorage.getItem(ACCESS_TOKEN_KEY);
}

function storeTokens(tokens: TokenPair) {
  localStorage.setItem(ACCESS_TOKEN_KEY, tokens.access_token);
  localStorage.setItem(REFRESH_TOKEN_KEY, tokens.refresh_token);
  // expires_in is seconds from now
  const expiresAt = Date.now() + tokens.expires_in * 1000;
  localStorage.setItem(EXPIRES_AT_KEY, String(expiresAt));
}

function clearTokens() {
  localStorage.removeItem(ACCESS_TOKEN_KEY);
  localStorage.removeItem(REFRESH_TOKEN_KEY);
  localStorage.removeItem(EXPIRES_AT_KEY);
}

function isTokenExpiringSoon(): boolean {
  const expiresAt = Number(localStorage.getItem(EXPIRES_AT_KEY) ?? "0");
  // Refresh if < 2 minutes remain
  return Date.now() > expiresAt - 2 * 60 * 1000;
}

// ─── Context types ────────────────────────────────────────────────────────────

export type AuthStatus =
  | "loading"       // initial check in progress
  | "unauthenticated"
  | "authenticated"
  | "setup_required"; // first-run, no users exist

export interface AuthUser extends MeResponse {}

interface AuthContextValue {
  status: AuthStatus;
  user: AuthUser | null;
  login: (username: string, password: string) => Promise<void>;
  setup: (username: string, password: string) => Promise<void>;
  logout: () => Promise<void>;
  /** Silently refreshes the access token; called by the API layer on 401. */
  refreshTokens: () => Promise<string | null>;
}

const AuthContext = createContext<AuthContextValue | null>(null);

// ─── Provider ─────────────────────────────────────────────────────────────────

export function AuthProvider({ children }: { children: React.ReactNode }) {
  const [status, setStatus] = useState<AuthStatus>("loading");
  const [user, setUser] = useState<AuthUser | null>(null);
  // In-flight refresh promise — prevents concurrent refresh races
  const refreshPromiseRef = useRef<Promise<string | null> | null>(null);

  const fetchMe = useCallback(async (): Promise<boolean> => {
    try {
      const me = await authApi.me();
      setUser(me);
      setStatus("authenticated");
      return true;
    } catch {
      clearTokens();
      setUser(null);
      setStatus("unauthenticated");
      return false;
    }
  }, []);

  // Initialise on mount: check setup status, then try existing token
  useEffect(() => {
    let cancelled = false;

    async function init() {
      try {
        const { setup_required } = await authApi.getSetupStatus();
        if (cancelled) return;

        if (setup_required) {
          setStatus("setup_required");
          return;
        }
      } catch {
        // Backend unreachable — fall through to unauthenticated
        if (!cancelled) setStatus("unauthenticated");
        return;
      }

      // Setup done — check if we have a valid / refreshable token
      const accessToken = getStoredAccessToken();
      if (!accessToken) {
        if (!cancelled) setStatus("unauthenticated");
        return;
      }

      if (isTokenExpiringSoon()) {
        const refreshToken = localStorage.getItem(REFRESH_TOKEN_KEY);
        if (refreshToken) {
          try {
            const tokens = await authApi.refresh(refreshToken);
            if (cancelled) return;
            storeTokens(tokens);
          } catch {
            if (!cancelled) setStatus("unauthenticated");
            return;
          }
        }
      }

      if (!cancelled) await fetchMe();
    }

    init();
    return () => { cancelled = true; };
  }, [fetchMe]);

  const login = useCallback(
    async (username: string, password: string) => {
      const tokens = await authApi.login({ username, password });
      storeTokens(tokens);
      await fetchMe();
    },
    [fetchMe],
  );

  const setup = useCallback(
    async (username: string, password: string) => {
      const tokens = await authApi.setup({ username, password });
      storeTokens(tokens);
      await fetchMe();
    },
    [fetchMe],
  );

  const logout = useCallback(async () => {
    try {
      await authApi.logout();
    } catch {
      // Best-effort — always clear locally
    } finally {
      clearTokens();
      setUser(null);
      setStatus("unauthenticated");
    }
  }, []);

  const refreshTokens = useCallback(async (): Promise<string | null> => {
    // Deduplicate concurrent refresh calls
    if (refreshPromiseRef.current) return refreshPromiseRef.current;

    const doRefresh = async (): Promise<string | null> => {
      const refreshToken = localStorage.getItem(REFRESH_TOKEN_KEY);
      if (!refreshToken) {
        setStatus("unauthenticated");
        return null;
      }
      try {
        const tokens = await authApi.refresh(refreshToken);
        storeTokens(tokens);
        return tokens.access_token;
      } catch {
        clearTokens();
        setUser(null);
        setStatus("unauthenticated");
        return null;
      } finally {
        refreshPromiseRef.current = null;
      }
    };

    refreshPromiseRef.current = doRefresh();
    return refreshPromiseRef.current;
  }, []);

  return (
    <AuthContext.Provider
      value={{ status, user, login, setup, logout, refreshTokens }}
    >
      {children}
    </AuthContext.Provider>
  );
}

// ─── Hook ─────────────────────────────────────────────────────────────────────

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within <AuthProvider>");
  return ctx;
}
