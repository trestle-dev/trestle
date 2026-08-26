export type TrestleError = { error: { code: string; message: string; requestId?: string; details?: unknown[] } };
export type RecordValues = Record<string, unknown>;
export type TrestleRecord = { id: string; version: number; createdAt: string; updatedAt: string; values: RecordValues };

export class TrestleClient {
  constructor(private baseURL: string, private token?: string) {}
  private async request<T>(path: string, init: RequestInit = {}): Promise<T> {
    const response = await fetch(new URL(path, this.baseURL), { ...init, headers: { Accept: "application/json", "Content-Type": "application/json", ...(this.token ? { Authorization: `Bearer ${this.token}` } : {}), ...init.headers } });
    const body = await response.json();
    if (!response.ok) throw Object.assign(new Error(body?.error?.message || "Trestle request failed"), { response, body });
    return body as T;
  }
  capabilities() { return this.request<Record<string, unknown>>("/api/v1/capabilities"); }
  list(collection: string, query = "") { return this.request<{items:TrestleRecord[];nextCursor:string}>(`/api/v1/collections/${encodeURIComponent(collection)}/records${query}`); }
  create(collection: string, values: RecordValues) { return this.request<TrestleRecord>(`/api/v1/collections/${encodeURIComponent(collection)}/records`, { method: "POST", body: JSON.stringify({ values }) }); }
}
