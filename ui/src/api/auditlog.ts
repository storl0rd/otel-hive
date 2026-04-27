import { apiGet } from "./base";

export interface AuditEntry {
  id: number;
  timestamp: string;
  actor_id?: string;
  actor_name?: string;
  event_type: string;
  resource_type?: string;
  resource_id?: string;
  details?: string;
  ip_address?: string;
}

export interface AuditLogResponse {
  entries: AuditEntry[];
  total: number;
  page: number;
  limit: number;
}

export const listAuditLog = (
  page = 1,
  limit = 50,
  eventType?: string,
): Promise<AuditLogResponse> => {
  const params = new URLSearchParams({
    page: String(page),
    limit: String(limit),
  });
  if (eventType) params.set("event_type", eventType);
  return apiGet<AuditLogResponse>(`/audit-log?${params.toString()}`);
};
