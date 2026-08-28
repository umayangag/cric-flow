/**
 * The shape go-app actually returns when a request fails.
 *
 * Every error response from the API is `{code, message, hint?}` — the `hint` written
 * specifically to tell the operator what to do next ("run this pipeline step", "pass
 * the format you want"). Some carry `available` too, which is the whole answer when a
 * model is not loaded: knowing *what is* loaded beats knowing that something is not.
 *
 * This type exists because that payload had nowhere to live. `httpApi` threw
 * `new Error("HTTP 400 Bad Request: {…json…}")`, so the fields were stringified into a
 * message and every surface rendered the whole thing in a red box, hint included but
 * unreadable. Parsing here is what lets W1-2 render each part as what it is.
 */
export class ApiError extends Error {
  /** HTTP status, when the failure was a response rather than a transport error. */
  readonly status?: number;
  /** Machine-readable reason, e.g. MODEL_NOT_LOADED. Empty for transport failures. */
  readonly code?: string;
  /** What to do about it, in the backend's own words. Render this; do not summarise it. */
  readonly hint?: string;
  /** What *is* available, when the failure is that something named was not. */
  readonly available?: string[];

  constructor(
    message: string,
    init: { status?: number; code?: string; hint?: string; available?: string[] } = {},
  ) {
    super(message);
    this.name = 'ApiError';
    this.status = init.status;
    this.code = init.code;
    this.hint = init.hint;
    this.available = init.available;
  }
}

/**
 * Coerce anything thrown into an ApiError.
 *
 * Async code can reject with anything, and a surface that renders "[object Object]"
 * is no better than one that renders nothing. `fallback` is what to say when the
 * throw carried no message of its own.
 */
export function toApiError(thrown: unknown, fallback = 'Request failed'): ApiError {
  if (thrown instanceof ApiError) return thrown;
  if (thrown instanceof Error) return new ApiError(thrown.message || fallback);
  if (typeof thrown === 'string' && thrown.trim() !== '') return new ApiError(thrown);
  return new ApiError(fallback);
}
