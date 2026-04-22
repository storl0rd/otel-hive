import { simpleRequest } from "./base";

// ─── Types ────────────────────────────────────────────────────────────────────

export interface SetupStatusResponse {
  setup_required: boolean;
}

export interface TokenPair {
  access_token: string;
  refresh_token: string;
  expires_in: number;
}

export interface LoginRequest {
  username: string;
  password: string;
}

export interface SetupRequest {
  username: string;
  password: string;
}

export interface MeResponse {
  id: string;
  username: string;
  role: "admin" | "operator" | "viewer";
  created_at: string;
}

export interface ApiKey {
  id: string;
  name: string;
  created_at: string;
  last_used_at: string | null;
}

export interface CreateApiKeyRequest {
  name: string;
}

export interface CreateApiKeyResponse {
  api_key: ApiKey;
  key: string; // plaintext — shown once only
}

// ─── API calls ────────────────────────────────────────────────────────────────

export const authApi = {
  getSetupStatus(): Promise<SetupStatusResponse> {
    return simpleRequest<SetupStatusResponse>("/api/auth/setup/status");
  },

  setup(req: SetupRequest): Promise<TokenPair> {
    return simpleRequest<TokenPair>("/api/auth/setup", {
      method: "POST",
      body: JSON.stringify(req),
    });
  },

  login(req: LoginRequest): Promise<TokenPair> {
    return simpleRequest<TokenPair>("/api/auth/login", {
      method: "POST",
      body: JSON.stringify(req),
    });
  },

  logout(): Promise<void> {
    return simpleRequest<void>("/api/auth/logout", { method: "POST" });
  },

  refresh(refreshToken: string): Promise<TokenPair> {
    return simpleRequest<TokenPair>("/api/auth/refresh", {
      method: "POST",
      body: JSON.stringify({ refresh_token: refreshToken }),
    });
  },

  me(): Promise<MeResponse> {
    return simpleRequest<MeResponse>("/api/auth/me");
  },

  listApiKeys(): Promise<{ api_keys: ApiKey[] }> {
    return simpleRequest<{ api_keys: ApiKey[] }>("/api/auth/api-keys");
  },

  createApiKey(req: CreateApiKeyRequest): Promise<CreateApiKeyResponse> {
    return simpleRequest<CreateApiKeyResponse>("/api/auth/api-keys", {
      method: "POST",
      body: JSON.stringify(req),
    });
  },

  revokeApiKey(id: string): Promise<void> {
    return simpleRequest<void>(`/api/auth/api-keys/${id}`, {
      method: "DELETE",
    });
  },
};
