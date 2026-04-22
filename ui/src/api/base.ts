import { apiBaseUrl } from "../config";

// ─── Types ────────────────────────────────────────────────────────────────────

export interface ApiResponse<T = unknown> {
  success?: boolean;
  error?: string;
  data?: T;
}

export const apiConfig = {
  baseUrl: apiBaseUrl,
  defaultHeaders: {
    "Content-Type": "application/json",
  },
};

// ─── Token helpers ────────────────────────────────────────────────────────────
// Imported lazily to avoid circular dependencies (useAuth imports base.ts
// indirectly via the auth API client).

function getAccessToken(): string | null {
  return localStorage.getItem("otel_hive_access_token");
}

// refreshTokens is set by AuthProvider after it mounts so the API layer can
// trigger a refresh without a direct import cycle.
let _refreshTokens: (() => Promise<string | null>) | null = null;

export function registerRefreshFn(fn: () => Promise<string | null>) {
  _refreshTokens = fn;
}

// ─── Core request ─────────────────────────────────────────────────────────────

/**
 * Low-level fetch wrapper.
 *
 * Automatically:
 * - Adds `Authorization: Bearer <token>` when a token is present
 * - On 401: attempts one silent token refresh then retries
 * - On 503 `setup_required`: navigates to /setup
 */
export const simpleRequest = async <T = unknown>(
  endpoint: string,
  options: RequestInit = {},
): Promise<T> => {
  const url = `${apiConfig.baseUrl}${endpoint}`;

  const buildHeaders = (token?: string | null): Record<string, string> => {
    const headers: Record<string, string> = { ...apiConfig.defaultHeaders };
    const t = token ?? getAccessToken();
    if (t) headers["Authorization"] = `Bearer ${t}`;
    return { ...headers, ...(options.headers as Record<string, string>) };
  };

  const doFetch = (token?: string | null) =>
    fetch(url, {
      ...options,
      headers: buildHeaders(token),
    });

  let response = await doFetch();

  // ── 401: try refresh once ────────────────────────────────────────────────
  if (response.status === 401 && _refreshTokens) {
    const newToken = await _refreshTokens();
    if (newToken) {
      response = await doFetch(newToken);
    }
  }

  // ── 503 setup_required: redirect to /setup ───────────────────────────────
  if (response.status === 503) {
    try {
      const body = await response.clone().json();
      if (body?.setup_required) {
        window.location.href = "/setup";
        // Return a never-resolving promise — the page is navigating away
        return new Promise<T>(() => {});
      }
    } catch {
      // Not a setup_required 503 — fall through to error handling
    }
  }

  if (!response.ok) {
    let errorMessage = `API request failed: ${response.status} ${response.statusText}`;
    try {
      const errorData = await response.json();
      if (errorData.error) {
        errorMessage = errorData.error;
        if (errorData.details) errorMessage += `: ${errorData.details}`;
      }
    } catch {
      // Use default message
    }
    const error = new Error(errorMessage) as Error & { status: number };
    error.status = response.status;
    throw error;
  }

  // 204 No Content
  if (response.status === 204) return undefined as unknown as T;

  return response.json();
};

// ─── HTTP method helpers ──────────────────────────────────────────────────────

export const apiGet = <T = unknown>(
  endpoint: string,
  params?: Record<string, string>,
): Promise<T> => {
  const url = params ? `${endpoint}?${new URLSearchParams(params)}` : endpoint;
  return simpleRequest<T>(url, { method: "GET" });
};

export const apiPost = <T = unknown>(
  endpoint: string,
  data?: unknown,
): Promise<T> =>
  simpleRequest<T>(endpoint, {
    method: "POST",
    body: data ? JSON.stringify(data) : undefined,
  });

export const apiPut = <T = unknown>(
  endpoint: string,
  data?: unknown,
): Promise<T> =>
  simpleRequest<T>(endpoint, {
    method: "PUT",
    body: data ? JSON.stringify(data) : undefined,
  });

export const apiDelete = <T = unknown>(endpoint: string): Promise<T> =>
  simpleRequest<T>(endpoint, { method: "DELETE" });

export const apiPatch = <T = unknown>(
  endpoint: string,
  data?: unknown,
): Promise<T> =>
  simpleRequest<T>(endpoint, {
    method: "PATCH",
    body: data ? JSON.stringify(data) : undefined,
  });
