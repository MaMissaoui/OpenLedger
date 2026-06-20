// Wire types mirroring the OpenLedger API (openapi/openapi.yaml). Money is
// always an exact { num, denom } pair — never a float.

export interface Numeric {
  num: number;
  denom: number;
}

export interface Tokens {
  accessToken: string;
  refreshToken: string;
  tokenType: string;
  expiresIn: number;
}

export interface Book {
  guid: string;
  rootAccountGuid: string;
}

export interface Account {
  guid: string;
  name: string;
  type: string;
  commodityGuid: string;
  parentGuid: string;
  code: string;
  description: string;
  placeholder: boolean;
  // Present on the chart-of-accounts list; absent on the create response. Sum of
  // the account's own splits in its commodity (no subtree roll-up).
  balance?: Numeric;
  // Own balance plus all same-commodity descendants — the roll-up shown against
  // placeholder parents and section totals. List-only, like balance.
  subtreeBalance?: Numeric;
}

export interface Commodity {
  guid: string;
  namespace: string;
  mnemonic: string;
  fraction: number;
}

export interface Price {
  guid: string;
  commodityGuid: string;
  currencyGuid: string;
  date: string;
  source: string;
  type: string;
  value: Numeric;
}

// Result of importing a GnuCash file: the new book's GUID and the counts of
// objects created, returned by POST /api/v1/imports/gnucash.
export interface ImportResult {
  bookGuid: string;
  commodities: number;
  accounts: number;
  transactions: number;
}

// Result of importing an OFX/QIF/CSV bank statement into an account.
export interface BankImportResult {
  accountGuid: string;
  imported: number;
  skipped: number;
}

// First rows of an uploaded CSV, returned by the preview endpoint to drive the
// column-mapping wizard.
export interface CsvPreview {
  rows: string[][];
  totalRows: number;
  columns: number;
}

// One account's natural-sign balance within a report section.
export interface ReportLine {
  account: Account;
  balance: Numeric;
}

// A group of report lines plus their total, both in natural sign (positive for
// the account type's normal direction). Zero-balance accounts are omitted.
export interface ReportSection {
  lines: ReportLine[];
  total: Numeric;
}

// Point-in-time statement of financial position. For a balanced single-currency
// book, assets.total === totalLiabilitiesAndEquity.
export interface BalanceSheet {
  bookGuid: string;
  asOf: string;
  assets: ReportSection;
  liabilities: ReportSection;
  equity: ReportSection;
  retainedEarnings: Numeric;
  totalLiabilitiesAndEquity: Numeric;
}

// Statement of performance over [from, to]. Single-currency only.
export interface IncomeStatement {
  bookGuid: string;
  from: string;
  to: string;
  income: ReportSection;
  expense: ReportSection;
  netIncome: Numeric;
}

// One account's cash effect for the period: inflow positive, outflow negative
// (the negative of the account's own balance change).
export interface CashFlowLine {
  account: Account;
  amount: Numeric;
}

// A group of cash-flow lines with their total. Zero-effect accounts are omitted.
export interface CashFlowSection {
  lines: CashFlowLine[];
  total: Numeric;
}

// Movement of cash over [from, to], grouped into operating, investing, and
// financing activities. netChange = operating + investing + financing, and
// equals endingCash − beginningCash. Single-currency only.
export interface CashFlowStatement {
  bookGuid: string;
  from: string;
  to: string;
  operating: CashFlowSection;
  investing: CashFlowSection;
  financing: CashFlowSection;
  netChange: Numeric;
  beginningCash: Numeric;
  endingCash: Numeric;
}

// Projected cash at the end of one month, with that month's flows.
export interface ForecastPoint {
  date: string;
  projectedCash: Numeric;
  inflow: Numeric;
  outflow: Numeric;
}

// One projected cash movement — a single scheduled-transaction occurrence, with
// its net cash effect (inflow positive, outflow negative).
export interface ForecastEvent {
  date: string;
  name: string;
  amount: Numeric;
}

// Forward projection of cash, simulating the book's scheduled transactions.
export interface CashFlowForecast {
  bookGuid: string;
  from: string;
  to: string;
  startingCash: Numeric;
  endingCash: Numeric;
  netChange: Numeric;
  lowestCash: Numeric;
  lowestDate: string;
  points: ForecastPoint[];
  events: ForecastEvent[];
}

// One security position in the portfolio report. shares is in the account's
// commodity; the money fields are in the quote currency. When hasPrice is false
// the holding has no quote and the market-value fields are absent.
export interface Holding {
  account: Account;
  shares: Numeric;
  costBasis: Numeric;
  hasPrice: boolean;
  price?: Numeric;
  priceCurrencyGuid?: string;
  marketValue?: Numeric;
  unrealizedGain?: Numeric;
}

export interface Portfolio {
  bookGuid: string;
  holdings: Holding[];
}

// A security buy/sell request. shares is in the security's commodity; cash is the
// total paid (buy) or received (sell) in the cash account's currency.
export interface TradeInput {
  securityAccountGuid: string;
  cashAccountGuid: string;
  shares: Numeric;
  cash: Numeric;
  description?: string;
}

export interface TradeResult {
  transactionGuid: string;
  realizedGain: Numeric;
}

// One realized gain/loss line (natural sign: positive is a gain).
export interface CapitalGainLine {
  date: string;
  description: string;
  account: string;
  amount: Numeric;
}

export interface CapitalGainsReport {
  bookGuid: string;
  from: string;
  to: string;
  lines: CapitalGainLine[];
  total: Numeric;
}

export interface TransactionSplit {
  guid: string;
  accountGuid: string;
  memo: string;
  action: string;
  value: Numeric;
  quantity: Numeric;
}

// A full transaction with every split — what the edit UI loads (a single
// account's register only carries that account's own split).
export interface Transaction {
  guid: string;
  currencyGuid: string;
  num: string;
  postDate: string;
  description: string;
  splits: TransactionSplit[];
}

// One template split in a scheduled transaction.
export interface ScheduledSplit {
  guid?: string;
  accountGuid: string;
  memo: string;
  value: Numeric;
}

// A recurring posting template. nextDueDate is computed server-side.
export interface ScheduledTransaction {
  guid: string;
  bookGuid: string;
  name: string;
  description: string;
  enabled: boolean;
  currencyGuid: string;
  period: "once" | "daily" | "weekly" | "monthly" | "yearly";
  every: number;
  startDate: string;
  endDate?: string;
  lastPostedDate?: string;
  nextDueDate?: string;
  splits: ScheduledSplit[];
}

// Request body to create/update a scheduled transaction.
export interface NewScheduledTransaction {
  name: string;
  description?: string;
  enabled: boolean;
  currencyGuid: string;
  period: "once" | "daily" | "weekly" | "monthly" | "yearly";
  every: number;
  startDate: string;
  endDate?: string;
  splits: ScheduledSplit[];
}

// One entry in the post-due result.
export interface PostedSchedule {
  scheduleGuid: string;
  name: string;
  postDate: string;
  txGuid: string;
}

export interface BudgetAmount {
  accountGuid: string;
  periodNum: number;
  value: Numeric;
}

export interface Budget {
  guid: string;
  bookGuid: string;
  name: string;
  description: string;
  periodType: "monthly" | "quarterly" | "yearly";
  numPeriods: number;
  startDate: string;
  amounts: BudgetAmount[];
}

export interface NewBudget {
  name: string;
  description: string;
  periodType: "monthly" | "quarterly" | "yearly";
  numPeriods: number;
  startDate: string;
  amounts: BudgetAmount[];
}

export interface BudgetVarianceLine {
  account: Account;
  budgeted: Numeric;
  actual: Numeric;
  variance: Numeric;
}

export interface BudgetReport {
  budgetGuid: string;
  periodNum: number;
  periodLabel: string;
  periodStart: string;
  periodEnd: string;
  lines: BudgetVarianceLine[];
  totalBudgeted: Numeric;
  totalActual: Numeric;
  totalVariance: Numeric;
}

export interface Address {
  name: string;
  addr1: string;
  addr2: string;
  phone: string;
  email: string;
}

export interface Customer {
  guid: string;
  bookGuid: string;
  name: string;
  id: string;
  notes: string;
  active: boolean;
  currencyGuid: string;
  addr: Address;
  creditLimit: Numeric;
  termsGuid: string;
  createdAt: string;
}

export interface NewCustomer {
  name: string;
  id?: string;
  notes?: string;
  active?: boolean;
  currencyGuid: string;
  addr?: Partial<Address>;
  creditLimit?: Numeric;
  termsGuid?: string;
}

export interface Vendor {
  guid: string;
  bookGuid: string;
  name: string;
  id: string;
  notes: string;
  active: boolean;
  currencyGuid: string;
  addr: Address;
  termsGuid: string;
  createdAt: string;
}

export interface NewVendor {
  name: string;
  id?: string;
  notes?: string;
  active?: boolean;
  currencyGuid: string;
  addr?: Partial<Address>;
  termsGuid?: string;
}

export interface NewEntry {
  date?: string;
  description?: string;
  action?: string;
  notes?: string;
  quantity?: Numeric;
  accountGuid: string;
  price: Numeric;
  taxable?: boolean;
  taxTableGuid?: string;
}

export interface Entry extends NewEntry {
  guid: string;
  invoiceGuid: string;
  lineTotal: Numeric;
  createdAt: string;
}

export interface NewInvoice {
  id?: string;
  type: "invoice" | "bill";
  ownerGuid: string;
  dateOpened?: string;
  notes?: string;
  active?: boolean;
  currencyGuid: string;
  termsGuid?: string;
}

export interface Invoice extends NewInvoice {
  guid: string;
  bookGuid: string;
  datePosted: string | null;
  dateDue: string | null;
  postTxnGuid: string;
  postAccGuid: string;
  paidAt: string | null;
  paidTxnGuid: string;
  createdAt: string;
  entries?: Entry[];
}

export interface AgingReportRow {
  invoice: Invoice;
  total: Numeric;
  daysOverdue: number;
}

export interface AgingBucket {
  label: string;
  rows: AgingReportRow[];
  total: Numeric;
}

export interface AgingReport {
  bookGuid: string;
  asOf: string;
  buckets: AgingBucket[];
  total: Numeric;
}

export interface RegisterEntry {
  splitGuid: string;
  txGuid: string;
  postDate: string;
  description: string;
  memo: string;
  reconcile: string;
  value: Numeric;
  quantity: Numeric;
  balance: Numeric;
}

export interface RegisterPage {
  accountGuid: string;
  total: number;
  limit: number;
  offset: number;
  entries: RegisterEntry[];
}

// ── Bill / payment terms ──────────────────────────────────────────────────────

export interface NewBillTerm {
  name: string;
  description?: string;
  type: "days" | "proximo";
  dueDays?: number;
  discountDays?: number;
  discount?: Numeric;
  cutoff?: number;
}

export interface BillTerm extends NewBillTerm {
  guid: string;
  bookGuid: string;
}

// ── Tax tables ────────────────────────────────────────────────────────────────

export interface TaxTableEntry {
  accountGuid: string;
  type: "percentage" | "value";
  amount: Numeric;
}

export interface NewTaxTable {
  name: string;
  entries: TaxTableEntry[];
}

export interface TaxTable extends NewTaxTable {
  guid: string;
  bookGuid: string;
}

// Account types the UI offers when creating an account, grouped for the picker.
export const ACCOUNT_TYPES = [
  "ASSET",
  "BANK",
  "CASH",
  "CREDIT",
  "LIABILITY",
  "INCOME",
  "EXPENSE",
  "EQUITY",
  "RECEIVABLE",
  "PAYABLE",
  "STOCK",
  "MUTUAL",
] as const;

// The five top-level buckets a chart of accounts rolls up into, in the order a
// ledger conventionally lists them.
export const TOP_LEVEL_ORDER = ["ASSET", "LIABILITY", "EQUITY", "INCOME", "EXPENSE"] as const;
