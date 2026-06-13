import type { Account, Book, Commodity, Numeric, Price, RegisterPage } from "./types";

// ApiError carries the HTTP status so callers can branch (e.g. 422 unbalanced)
// and a human message from the API's { error } body.
export class ApiError extends Error {
  constructor(
    public status: number,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

async function parseError(res: Response): Promise<ApiError> {
  let message = res.statusText;
  try {
    const body = (await res.json()) as { error?: string };
    if (body.error) message = body.error;
  } catch {
    // non-JSON body; keep statusText
  }
  return new ApiError(res.status, message);
}

// request performs an authenticated JSON request. Authentication is handled by
// Authelia via Traefik forward-auth; the browser's session cookie is sent
// automatically (same-origin). A 401 means the Authelia session has expired —
// the browser is redirected to the Authelia login portal.
async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  if (!(init.body instanceof FormData)) {
    headers.set("Content-Type", "application/json");
  }

  const res = await fetch(path, { ...init, headers });

  if (res.status === 401) {
    // Authelia session expired — redirect to the login portal.
    const portalUrl = import.meta.env.VITE_AUTHELIA_PORTAL_URL ?? "http://auth.openledger.localhost";
    window.location.href = `${portalUrl}/?rd=${encodeURIComponent(window.location.href)}`;
    throw new ApiError(401, "Session expired");
  }
  if (!res.ok) throw await parseError(res);
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

function post<T>(path: string, body: unknown): Promise<T> {
  return request<T>(path, { method: "POST", body: JSON.stringify(body) });
}

export const api = {
  listBooks: () => request<{ books: Book[] }>("/api/v1/books").then((r) => r.books),
  createBook: () => post<Book>("/api/v1/books", {}),
  listCommodities: () =>
    request<{ commodities: Commodity[] }>("/api/v1/commodities").then((r) => r.commodities),
  createCommodity: (mnemonic: string, fraction: number, fullname?: string) =>
    post<Commodity>("/api/v1/commodities", { mnemonic, fraction, fullname }),
  listAccounts: (bookGuid: string) =>
    request<{ bookGuid: string; accounts: Account[] }>(
      `/api/v1/books/${bookGuid}/accounts`,
    ).then((r) => r.accounts),
  createAccount: (input: {
    bookGuid: string;
    name: string;
    type: string;
    commodityGuid: string;
    parentGuid?: string;
    code?: string;
    placeholder?: boolean;
  }) => post<Account>("/api/v1/accounts", input),
  getRegister: (accountGuid: string) =>
    request<RegisterPage>(`/api/v1/accounts/${accountGuid}/register?limit=200`),
  postTransaction: (input: {
    currencyGuid: string;
    description: string;
    postDate?: string;
    splits: { accountGuid: string; value: Numeric; quantity: Numeric }[];
  }) => post<{ guid: string }>("/api/v1/transactions", input),
  listPrices: (commodityGuid: string) =>
    request<{ prices: Price[] }>(
      `/api/v1/prices?commodity=${encodeURIComponent(commodityGuid)}`,
    ).then((r) => r.prices),
  createPrice: (input: {
    commodityGuid: string;
    currencyGuid: string;
    value: Numeric;
    date?: string;
    source?: string;
    type?: string;
  }) => post<Price>("/api/v1/prices", input),
};
