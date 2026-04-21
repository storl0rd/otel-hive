// Phase 1 stub — telemetry API removed; will be rewired in Phase 2
// Components that import from here will compile but return empty data.

export interface MetricData {
  metric_name: string;
  value: number;
  timestamp: string;
  agent_id?: string;
  metric_attributes?: Record<string, unknown>;
}

export interface LogData {
  log_id: string;
  timestamp: string;
  body: string;
  severity: string;
  severity_text?: string;
  agent_id?: string;
  attributes?: Record<string, unknown>;
  log_attributes?: Record<string, unknown>;
}

export interface MetricsQuery {
  agent_id?: string;
  group_id?: string;
  start_time?: string;
  end_time?: string;
  limit?: number;
}

export interface LogsQuery {
  agent_id?: string;
  group_id?: string;
  start_time?: string;
  end_time?: string;
  limit?: number;
  severity?: string;
}

export async function queryMetrics(
  _query: MetricsQuery,
): Promise<{ metrics: MetricData[] }> {
  return { metrics: [] };
}

export async function queryLogs(
  _query: LogsQuery,
): Promise<{ logs: LogData[] }> {
  return { logs: [] };
}
