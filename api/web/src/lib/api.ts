// Typed client for the Family API. Authentication is an HttpOnly cookie set by
// the API: the page never sees a token. Every request carries the
// X-Requested-With header, which the API requires for cookie-authenticated
// calls (a cross-site form cannot set it, so this blocks CSRF).

/** An error answered by the API, with its stable code (see err.* texts). */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

/** Events the session listens to, sent by any failing call. */
export const sessionEvents = {
  unauthorized: 'family:unauthorized',
  passwordChangeRequired: 'family:password-change-required',
} as const;

type Query = Record<string, string | number | undefined>;

export async function request<T>(method: string, path: string, body?: unknown, query?: Query): Promise<T> {
  let url = `/api/v1/${path}`;
  if (query) {
    const params = new URLSearchParams();
    for (const [key, value] of Object.entries(query)) {
      if (value !== undefined) params.set(key, String(value));
    }
    const text = params.toString();
    if (text) url += `?${text}`;
  }

  const headers: Record<string, string> = { Accept: 'application/json', 'X-Requested-With': 'family-web' };
  if (body !== undefined) headers['Content-Type'] = 'application/json';

  let response: Response;
  try {
    response = await fetch(url, {
      method,
      headers,
      credentials: 'same-origin',
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch {
    throw new ApiError(0, 'network', 'network error');
  }

  if (response.ok) {
    if (response.status === 204) return undefined as T;
    return (await response.json()) as T;
  }

  let code = `http_${response.status}`;
  let message = response.statusText;
  try {
    const failure = (await response.json()) as { error?: string; code?: string };
    code = failure.code ?? code;
    message = failure.error ?? message;
  } catch {
    // Not JSON (proxy error page...): keep the HTTP status.
  }
  // Login answers 401 for a wrong password: that is not "session expired".
  if (response.status === 401 && path !== 'auth/login') {
    window.dispatchEvent(new Event(sessionEvents.unauthorized));
  }
  if (response.status === 403 && code === 'password_change_required') {
    window.dispatchEvent(new Event(sessionEvents.passwordChangeRequired));
  }
  throw new ApiError(response.status, code, message);
}

export const api = {
  get: <T>(path: string, query?: Query) => request<T>('GET', path, undefined, query),
  post: <T>(path: string, body?: unknown) => request<T>('POST', path, body ?? {}),
  patch: <T>(path: string, body: unknown) => request<T>('PATCH', path, body),
  put: <T>(path: string, body: unknown) => request<T>('PUT', path, body),
  delete: (path: string) => request<void>('DELETE', path),
};
