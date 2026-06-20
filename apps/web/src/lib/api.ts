import type {
  Account,
  AgingReport,
  BalanceSheet,
  BankImportResult,
  BillTerm,
  CsvPreview,
  Book,
  Budget,
  BudgetReport,
  CapitalGainsReport,
  CashFlowForecast,
  CashFlowStatement,
  Commodity,
  Customer,
  Entry,
  ImportResult,
  IncomeStatement,
  Invoice,
  NewBillTerm,
  NewBudget,
  NewCustomer,
  NewEntry,
  NewInvoice,
  NewScheduledTransaction,
  NewTaxTable,
  NewVendor,
  Numeric,
  TaxTable,
  Portfolio,
  PostedSchedule,
  Price,
  RegisterPage,
  ScheduledTransaction,
  Transaction,
  TradeInput,
  TradeResult,
  Vendor,
} from "./types";

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
  createCommodity: (mnemonic: string, fraction: number, fullname?: string, namespace?: string) =>
    post<Commodity>("/api/v1/commodities", { mnemonic, fraction, fullname, namespace }),
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
  // Set a split's reconcile flag ("n" unmarked, "c" cleared, "y" reconciled).
  reconcileSplit: (splitGuid: string, state: string) =>
    request<{ splitGuid: string; state: string }>(`/api/v1/splits/${splitGuid}/reconcile`, {
      method: "PATCH",
      body: JSON.stringify({ state }),
    }),
  postTransaction: (input: {
    currencyGuid: string;
    description: string;
    postDate?: string;
    splits: { accountGuid: string; value: Numeric; quantity: Numeric }[];
  }) => post<{ guid: string }>("/api/v1/transactions", input),
  getTransaction: (guid: string) => request<Transaction>(`/api/v1/transactions/${guid}`),
  // Wholesale replacement of a transaction's fields and splits (PATCH), with the
  // same body shape as postTransaction; the server re-validates the balance.
  updateTransaction: (
    guid: string,
    input: {
      currencyGuid: string;
      description: string;
      postDate?: string;
      splits: { accountGuid: string; value: Numeric; quantity: Numeric }[];
    },
  ) =>
    request<{ guid: string; splits: number }>(`/api/v1/transactions/${guid}`, {
      method: "PATCH",
      body: JSON.stringify(input),
    }),
  deleteTransaction: (guid: string) =>
    request<void>(`/api/v1/transactions/${guid}`, { method: "DELETE" }),
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
  // Fetch a live exchange rate from the server's quote provider (Frankfurter)
  // and record it as a price. Both GUIDs must be CURRENCY commodities.
  fetchPrice: (commodityGuid: string, currencyGuid: string) =>
    post<Price>("/api/v1/prices/fetch", { commodityGuid, currencyGuid }),
  // Import an OFX/QIF/CSV bank statement into a currency account. format is
  // optional for OFX/QIF (sniffed) but "csv" with a mapping JSON is required for
  // CSV. Each line posts against the book's Imbalance account; duplicates are
  // skipped.
  importBankStatement: (accountGuid: string, file: File, format?: string, mapping?: string) => {
    const body = new FormData();
    body.append("file", file);
    if (format) body.append("format", format);
    if (mapping) body.append("mapping", mapping);
    return request<BankImportResult>(
      `/api/v1/accounts/${accountGuid}/import-bank`,
      { method: "POST", body },
    );
  },
  // Parse an uploaded CSV and return its first rows so the import wizard can
  // build a column mapping. Nothing is persisted.
  previewBankCsv: (accountGuid: string, file: File) => {
    const body = new FormData();
    body.append("file", file);
    return request<CsvPreview>(
      `/api/v1/accounts/${accountGuid}/import-bank/preview`,
      { method: "POST", body },
    );
  },
  getBalanceSheet: (bookGuid: string, asOf?: string) => {
    const q = asOf ? `?asOf=${encodeURIComponent(asOf)}` : "";
    return request<BalanceSheet>(`/api/v1/books/${bookGuid}/reports/balance-sheet${q}`);
  },
  getIncomeStatement: (bookGuid: string, from?: string, to?: string) => {
    const params = new URLSearchParams();
    if (from) params.set("from", from);
    if (to) params.set("to", to);
    const q = params.toString() ? `?${params}` : "";
    return request<IncomeStatement>(`/api/v1/books/${bookGuid}/reports/income-statement${q}`);
  },
  getCashFlow: (bookGuid: string, from?: string, to?: string) => {
    const params = new URLSearchParams();
    if (from) params.set("from", from);
    if (to) params.set("to", to);
    const q = params.toString() ? `?${params}` : "";
    return request<CashFlowStatement>(`/api/v1/books/${bookGuid}/reports/cash-flow${q}`);
  },
  getCashFlowForecast: (bookGuid: string, months = 6, from?: string) => {
    const params = new URLSearchParams({ months: String(months) });
    if (from) params.set("from", from);
    return request<CashFlowForecast>(
      `/api/v1/books/${bookGuid}/reports/cash-flow-forecast?${params}`,
    );
  },
  getPortfolio: (bookGuid: string) =>
    request<Portfolio>(`/api/v1/books/${bookGuid}/reports/portfolio`),
  getCapitalGains: (bookGuid: string, from?: string, to?: string) => {
    const params = new URLSearchParams();
    if (from) params.set("from", from);
    if (to) params.set("to", to);
    const q = params.toString() ? `?${params}` : "";
    return request<CapitalGainsReport>(`/api/v1/books/${bookGuid}/reports/capital-gains${q}`);
  },
  // Buy/sell a security. shares is in the security's commodity, cash in the cash
  // account's currency; the server opens/consumes cost-basis lots and (on a sell)
  // realizes a FIFO capital gain.
  buySecurity: (input: TradeInput) => post<TradeResult>("/api/v1/securities/buy", input),
  sellSecurity: (input: TradeInput) => post<TradeResult>("/api/v1/securities/sell", input),
  listScheduledTransactions: (bookGuid: string) =>
    request<{ bookGuid: string; scheduledTransactions: ScheduledTransaction[] }>(
      `/api/v1/books/${bookGuid}/scheduled-transactions`,
    ).then((r) => r.scheduledTransactions),
  createScheduledTransaction: (bookGuid: string, input: NewScheduledTransaction) =>
    post<ScheduledTransaction>(`/api/v1/books/${bookGuid}/scheduled-transactions`, input),
  updateScheduledTransaction: (guid: string, input: NewScheduledTransaction) =>
    request<ScheduledTransaction>(`/api/v1/scheduled-transactions/${guid}`, {
      method: "PATCH",
      body: JSON.stringify(input),
    }),
  deleteScheduledTransaction: (guid: string) =>
    request<void>(`/api/v1/scheduled-transactions/${guid}`, { method: "DELETE" }),
  postDueSchedules: (bookGuid: string, asOf?: string) => {
    const q = asOf ? `?asOf=${encodeURIComponent(asOf)}` : "";
    return post<{ bookGuid: string; posted: PostedSchedule[] }>(
      `/api/v1/books/${bookGuid}/scheduled-transactions/post-due${q}`,
      {},
    );
  },
  // URL of the GnuCash export for a book in the given format ("sqlite" by
  // default, or "xml"). It's a plain authenticated GET, so the browser can
  // download it directly via an <a download> (the same-origin Authelia session
  // cookie is sent automatically).
  exportGnuCashUrl: (bookGuid: string, format: "sqlite" | "xml" = "sqlite") =>
    `/api/v1/books/${bookGuid}/export/gnucash${format === "xml" ? "?format=xml" : ""}`,

  listBudgets: (bookGuid: string) =>
    request<{ bookGuid: string; budgets: Budget[] }>(`/api/v1/books/${bookGuid}/budgets`),
  createBudget: (bookGuid: string, input: NewBudget) =>
    post<Budget>(`/api/v1/books/${bookGuid}/budgets`, input),
  getBudget: (guid: string) => request<Budget>(`/api/v1/budgets/${guid}`),
  updateBudget: (guid: string, input: NewBudget) =>
    request<Budget>(`/api/v1/budgets/${guid}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input),
    }),
  deleteBudget: (guid: string) =>
    request<void>(`/api/v1/budgets/${guid}`, { method: "DELETE" }),
  budgetReport: (guid: string, asOf?: string) => {
    const q = asOf ? `?asOf=${encodeURIComponent(asOf)}` : "";
    return request<BudgetReport>(`/api/v1/budgets/${guid}/report${q}`);
  },

  listCustomers: (bookGuid: string, activeOnly = false) => {
    const q = activeOnly ? "?active=true" : "";
    return request<{ bookGuid: string; customers: Customer[] }>(
      `/api/v1/books/${bookGuid}/customers${q}`,
    ).then((r) => r.customers);
  },
  createCustomer: (bookGuid: string, input: NewCustomer) =>
    post<Customer>(`/api/v1/books/${bookGuid}/customers`, input),
  getCustomer: (guid: string) => request<Customer>(`/api/v1/customers/${guid}`),
  updateCustomer: (guid: string, input: NewCustomer) =>
    request<Customer>(`/api/v1/customers/${guid}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input),
    }),
  deleteCustomer: (guid: string) =>
    request<void>(`/api/v1/customers/${guid}`, { method: "DELETE" }),

  listVendors: (bookGuid: string, activeOnly = false) => {
    const q = activeOnly ? "?active=true" : "";
    return request<{ bookGuid: string; vendors: Vendor[] }>(
      `/api/v1/books/${bookGuid}/vendors${q}`,
    ).then((r) => r.vendors);
  },
  createVendor: (bookGuid: string, input: NewVendor) =>
    post<Vendor>(`/api/v1/books/${bookGuid}/vendors`, input),
  getVendor: (guid: string) => request<Vendor>(`/api/v1/vendors/${guid}`),
  updateVendor: (guid: string, input: NewVendor) =>
    request<Vendor>(`/api/v1/vendors/${guid}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input),
    }),
  deleteVendor: (guid: string) =>
    request<void>(`/api/v1/vendors/${guid}`, { method: "DELETE" }),

  listInvoices: (bookGuid: string, type: "invoice" | "bill" = "invoice") =>
    request<{ bookGuid: string; type: string; invoices: Invoice[] }>(
      `/api/v1/books/${bookGuid}/invoices?type=${type}`,
    ).then((r) => r.invoices),
  createInvoice: (bookGuid: string, input: NewInvoice) =>
    post<Invoice>(`/api/v1/books/${bookGuid}/invoices`, input),
  getInvoice: (guid: string) => request<Invoice>(`/api/v1/invoices/${guid}`),
  updateInvoice: (guid: string, input: NewInvoice) =>
    request<Invoice>(`/api/v1/invoices/${guid}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input),
    }),
  deleteInvoice: (guid: string) =>
    request<void>(`/api/v1/invoices/${guid}`, { method: "DELETE" }),
  postInvoice: (guid: string, postAccGuid: string, postDate?: string, dueDate?: string) =>
    post<Invoice>(`/api/v1/invoices/${guid}/post`, { postAccGuid, postDate, dueDate }),
  payInvoice: (guid: string, paymentAccGuid: string, paymentDate?: string) =>
    post<Invoice>(`/api/v1/invoices/${guid}/pay`, { paymentAccGuid, paymentDate }),
  arAgingReport: (bookGuid: string) =>
    request<AgingReport>(`/api/v1/books/${bookGuid}/reports/ar-aging`),
  apAgingReport: (bookGuid: string) =>
    request<AgingReport>(`/api/v1/books/${bookGuid}/reports/ap-aging`),

  listEntries: (invoiceGuid: string) =>
    request<{ invoiceGuid: string; entries: Entry[] }>(
      `/api/v1/invoices/${invoiceGuid}/entries`,
    ).then((r) => r.entries),
  addEntry: (invoiceGuid: string, input: NewEntry) =>
    post<Entry>(`/api/v1/invoices/${invoiceGuid}/entries`, input),
  updateEntry: (guid: string, input: NewEntry) =>
    request<Entry>(`/api/v1/entries/${guid}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input),
    }),
  deleteEntry: (guid: string) =>
    request<void>(`/api/v1/entries/${guid}`, { method: "DELETE" }),

  listBillTerms: (bookGuid: string) =>
    request<{ bookGuid: string; billTerms: BillTerm[] }>(
      `/api/v1/books/${bookGuid}/bill-terms`,
    ).then((r) => r.billTerms),
  createBillTerm: (bookGuid: string, input: NewBillTerm) =>
    post<BillTerm>(`/api/v1/books/${bookGuid}/bill-terms`, input),
  getBillTerm: (guid: string) => request<BillTerm>(`/api/v1/bill-terms/${guid}`),
  updateBillTerm: (guid: string, input: NewBillTerm) =>
    request<BillTerm>(`/api/v1/bill-terms/${guid}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input),
    }),
  deleteBillTerm: (guid: string) =>
    request<void>(`/api/v1/bill-terms/${guid}`, { method: "DELETE" }),

  listTaxTables: (bookGuid: string) =>
    request<{ bookGuid: string; taxTables: TaxTable[] }>(
      `/api/v1/books/${bookGuid}/tax-tables`,
    ).then((r) => r.taxTables),
  createTaxTable: (bookGuid: string, input: NewTaxTable) =>
    post<TaxTable>(`/api/v1/books/${bookGuid}/tax-tables`, input),
  getTaxTable: (guid: string) => request<TaxTable>(`/api/v1/tax-tables/${guid}`),
  updateTaxTable: (guid: string, input: NewTaxTable) =>
    request<TaxTable>(`/api/v1/tax-tables/${guid}`, {
      method: "PATCH",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(input),
    }),
  deleteTaxTable: (guid: string) =>
    request<void>(`/api/v1/tax-tables/${guid}`, { method: "DELETE" }),

  // Import a GnuCash file (SQLite or XML, optionally gzipped) as a new book.
  // The file is sent as multipart/form-data under the "file" field; `request`
  // detects the FormData body and skips the JSON content-type so the browser
  // sets the multipart boundary. Returns the new book GUID and the counts of
  // objects imported.
  importGnuCash: (file: File) => {
    const body = new FormData();
    body.append("file", file);
    return request<ImportResult>("/api/v1/imports/gnucash", { method: "POST", body });
  },
};
