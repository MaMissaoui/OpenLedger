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
