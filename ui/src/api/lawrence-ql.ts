// Phase 1 stub — Lawrence QL API removed; will be replaced in Phase 2
// Components that import from here will compile but are not reachable via routing.

export interface QueryResult {
  type: string;
  columns?: string[];
  rows?: unknown[][];
  labels: Record<string, string>;
  data: Record<string, unknown>;
  value: number;
  timestamp: string;
}

export interface LawrenceQLMeta {
  execution_time: number;
  row_count: number;
  query_type: string;
}

export interface LawrenceQLResponse {
  results: QueryResult[];
  error?: string;
  meta: LawrenceQLMeta;
}

export interface LawrenceQLRequest {
  query: string;
  [key: string]: unknown;
}

export interface QueryTemplate {
  id: string;
  name: string;
  description: string;
  query: string;
  category?: string;
}

export interface FunctionInfo {
  name: string;
  description: string;
  args?: string[];
}

export interface ValidationResult {
  valid: boolean;
  error?: string;
}

export interface SuggestionsResponse {
  suggestions: string[];
}

export async function executeLawrenceQL(
  _request: LawrenceQLRequest,
): Promise<LawrenceQLResponse> {
  return {
    results: [],
    meta: { execution_time: 0, row_count: 0, query_type: "unknown" },
  };
}

export async function validateQuery(
  _query: string,
): Promise<ValidationResult> {
  return { valid: true };
}

export async function getQuerySuggestions(
  _query: string,
  _cursorPos: number,
): Promise<SuggestionsResponse> {
  return { suggestions: [] };
}

export async function getQueryTemplates(): Promise<{
  templates: QueryTemplate[];
}> {
  return { templates: [] };
}

export async function getQueryFunctions(): Promise<{
  functions: FunctionInfo[];
}> {
  return { functions: [] };
}
