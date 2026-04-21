// Phase 1 stub — collector-metrics API removed; will be rewired in Phase 2
// Components that import from here will compile but return empty data.

export interface ComponentMetrics {
  component_id?: string;
  component_type: string;
  component_name: string;
  pipeline_type: string;
  throughput: number;
  errors: number;
  error_rate: number;
  received?: number;
  accepted?: number;
  refused?: number;
  dropped?: number;
  sent?: number;
  send_failed?: number;
  last_updated?: string;
  timestamp?: string;
  metrics?: Record<string, number>;
}

export async function fetchAgentComponentMetrics(
  _agentId: string,
  _minutes?: number,
): Promise<ComponentMetrics[]> {
  return [];
}
