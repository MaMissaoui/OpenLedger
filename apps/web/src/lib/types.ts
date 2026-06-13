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
}

export interface Commodity {
  guid: string;
  namespace: string;
  mnemonic: string;
  fraction: number;
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
] as const;

// The five top-level buckets a chart of accounts rolls up into, in the order a
// ledger conventionally lists them.
export const TOP_LEVEL_ORDER = ["ASSET", "LIABILITY", "EQUITY", "INCOME", "EXPENSE"] as const;
