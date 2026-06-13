import type { Numeric } from "./types";

// decimalsFor returns the number of decimal places implied by a denominator
// that is a power of ten (100 -> 2). Non-power-of-ten denominators fall back to
// a sensible 2.
function decimalsFor(denom: number): number {
  if (denom <= 1) return 0;
  const d = Math.round(Math.log10(denom));
  return Math.abs(10 ** d - denom) < 1e-9 ? d : 2;
}

export function toFloat(n: Numeric): number {
  return n.denom === 0 ? 0 : n.num / n.denom;
}

// formatMoney renders an exact amount at its natural scale with grouping, e.g.
// { num: -123456, denom: 100 } -> "−1,234.56" (true minus sign, never a hyphen).
export function formatMoney(n: Numeric): string {
  const places = decimalsFor(n.denom);
  const abs = Math.abs(toFloat(n)).toLocaleString(undefined, {
    minimumFractionDigits: places,
    maximumFractionDigits: places,
  });
  return n.num < 0 ? `−${abs}` : abs;
}

// parseAmount converts a user-entered decimal string into an exact Numeric at
// the given commodity fraction (default cents). Returns null if not parseable.
export function parseAmount(input: string, denom = 100): Numeric | null {
  const trimmed = input.trim().replace(/,/g, "");
  if (trimmed === "" || !/^-?\d*\.?\d+$/.test(trimmed)) return null;
  const cents = Math.round(parseFloat(trimmed) * denom);
  return { num: cents, denom };
}

export function negate(n: Numeric): Numeric {
  return { num: -n.num, denom: n.denom };
}
