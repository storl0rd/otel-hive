import { apiGet, apiPost, apiPut, apiDelete } from "./base";

export type SyncStatus = "pending" | "success" | "failed" | "";
export type ProviderType = "github" | "gitlab" | "gitea" | "http";

export interface GitSource {
  id: string;
  name: string;
  repo_url: string;
  has_token: boolean;
  branch: string;
  config_root: string;
  provider: ProviderType;
  poll_interval_seconds: number;
  has_webhook_secret: boolean;
  last_sync_sha?: string;
  last_sync_at?: string | null;
  last_sync_status: SyncStatus;
  last_sync_error?: string;
  created_at: string;
  updated_at: string;
}

export interface ListGitSourcesResponse {
  git_sources: GitSource[];
}

export interface CreateGitSourceRequest {
  name: string;
  repo_url: string;
  token?: string;
  branch?: string;
  config_root?: string;
  provider: ProviderType;
  poll_interval_seconds?: number;
  webhook_secret?: string;
}

export interface SyncResult {
  source_id: string;
  files_changed: number;
  agents_updated: number;
  error_count: number;
}

export const listGitSources = (): Promise<ListGitSourcesResponse> =>
  apiGet<ListGitSourcesResponse>("/git-sources");

export const getGitSource = (id: string): Promise<GitSource> =>
  apiGet<GitSource>(`/git-sources/${id}`);

export const createGitSource = (data: CreateGitSourceRequest): Promise<GitSource> =>
  apiPost<GitSource>("/git-sources", data);

export const updateGitSource = (
  id: string,
  data: CreateGitSourceRequest,
): Promise<GitSource> => apiPut<GitSource>(`/git-sources/${id}`, data);

export const deleteGitSource = (id: string): Promise<void> =>
  apiDelete<void>(`/git-sources/${id}`);

export const triggerSync = (id: string): Promise<SyncResult> =>
  apiPost<SyncResult>(`/git-sources/${id}/sync`, {});
