import { useEffect, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";

import { api } from "../lib/api";
import type { Account, BankImportResult, CsvPreview } from "../lib/types";

// Date-format presets offered by the CSV wizard, mapped to Go time layouts the
// server understands. "" lets the server try common layouts.
const DATE_FORMATS: { label: string; value: string }[] = [
  { label: "Auto-detect", value: "" },
  { label: "YYYY-MM-DD", value: "2006-01-02" },
  { label: "MM/DD/YYYY", value: "01/02/2006" },
  { label: "DD/MM/YYYY", value: "02/01/2006" },
  { label: "YYYY/MM/DD", value: "2006/01/02" },
];

// CsvMapping mirrors the server's mapping DTO (zero-based column indices).
interface CsvMapping {
  hasHeader: boolean;
  dateCol: number;
  dateFormat: string;
  descCols: number[];
  amountCol?: number;
  debitCol?: number;
  creditCol?: number;
  invert: boolean;
}

function isCsv(file: File): boolean {
  return file.name.toLowerCase().endsWith(".csv");
}

// guessColumn finds the first header matching any keyword (case-insensitive).
function guessColumn(headers: string[], keywords: string[]): number {
  const i = headers.findIndex((h) => keywords.some((k) => h.toLowerCase().includes(k)));
  return i;
}

export default function ImportStatementDialog({
  account,
  onClose,
}: {
  account: Account;
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const inputRef = useRef<HTMLInputElement>(null);
  const [file, setFile] = useState<File | null>(null);
  const [importing, setImporting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<BankImportResult | null>(null);

  // CSV wizard state.
  const [preview, setPreview] = useState<CsvPreview | null>(null);
  const [hasHeader, setHasHeader] = useState(true);
  const [dateCol, setDateCol] = useState(0);
  const [dateFormat, setDateFormat] = useState("");
  const [amountMode, setAmountMode] = useState<"single" | "debitcredit">("single");
  const [amountCol, setAmountCol] = useState(1);
  const [debitCol, setDebitCol] = useState(1);
  const [creditCol, setCreditCol] = useState(2);
  const [descCols, setDescCols] = useState<number[]>([]);
  const [invert, setInvert] = useState(false);

  const csv = file !== null && isCsv(file);

  // When a CSV is chosen, fetch a preview and guess sensible default columns.
  useEffect(() => {
    if (!file || !isCsv(file)) {
      setPreview(null);
      return;
    }
    let cancelled = false;
    setError(null);
    api.previewBankCsv(account.guid, file)
      .then((p) => {
        if (cancelled) return;
        setPreview(p);
        const headers = p.rows[0] ?? [];
        const date = guessColumn(headers, ["date"]);
        const amount = guessColumn(headers, ["amount", "value"]);
        const debit = guessColumn(headers, ["debit", "withdraw"]);
        const credit = guessColumn(headers, ["credit", "deposit"]);
        const desc = guessColumn(headers, ["desc", "memo", "payee", "narrative", "name"]);
        if (date >= 0) setDateCol(date);
        if (amount >= 0) setAmountCol(amount);
        if (debit >= 0 && credit >= 0) {
          setAmountMode("debitcredit");
          setDebitCol(debit);
          setCreditCol(credit);
        }
        if (desc >= 0) setDescCols([desc]);
      })
      .catch((e) => !cancelled && setError(e instanceof Error ? e.message : "Could not read CSV"));
    return () => { cancelled = true; };
  }, [file, account.guid]);

  const columnLabels: string[] =
    preview === null
      ? []
      : Array.from({ length: preview.columns }, (_, i) =>
          hasHeader ? preview.rows[0]?.[i] || `Column ${i + 1}` : `Column ${i + 1}`,
        );
  const dataRows = preview === null ? [] : hasHeader ? preview.rows.slice(1) : preview.rows;

  function toggleDescCol(i: number) {
    setDescCols((prev) => (prev.includes(i) ? prev.filter((c) => c !== i) : [...prev, i].sort((a, b) => a - b)));
  }

  async function handleImport() {
    if (!file) return;
    setError(null);
    setImporting(true);
    try {
      let mapping: string | undefined;
      let format: string | undefined;
      if (csv) {
        const m: CsvMapping = { hasHeader, dateCol, dateFormat, descCols, invert };
        if (amountMode === "single") m.amountCol = amountCol;
        else { m.debitCol = debitCol; m.creditCol = creditCol; }
        mapping = JSON.stringify(m);
        format = "csv";
      }
      const res = await api.importBankStatement(account.guid, file, format, mapping);
      setResult(res);
      qc.invalidateQueries({ queryKey: ["register", account.guid] });
      qc.invalidateQueries({ queryKey: ["accounts"] });
      qc.invalidateQueries({ queryKey: ["balance-sheet"] });
      qc.invalidateQueries({ queryKey: ["income-statement"] });
    } catch (e) {
      setError(e instanceof Error ? e.message : "Import failed");
    } finally {
      setImporting(false);
    }
  }

  const colSelect = (value: number, onChange: (n: number) => void) => (
    <select value={value} onChange={(e) => onChange(Number(e.target.value))}>
      {columnLabels.map((label, i) => (
        <option key={i} value={i}>{label}</option>
      ))}
    </select>
  );

  return (
    <div className="dialog-overlay" onClick={onClose}>
      <div className="dialog" onClick={(e) => e.stopPropagation()} style={csv && !result ? { maxWidth: "44rem" } : undefined}>
        <div className="dialog__header">
          <h2>Import Statement — {account.name}</h2>
          <button className="dialog__close" onClick={onClose} aria-label="Close">×</button>
        </div>
        <div className="dialog__body">
          {error && <p className="error" style={{ margin: 0 }}>{error}</p>}

          {result ? (
            <>
              <p style={{ margin: 0 }}>Statement imported.</p>
              <div className="import-result">
                <div className="import-result__stat">
                  <span className="import-result__num">{result.imported}</span>
                  <span className="import-result__label">Imported</span>
                </div>
                <div className="import-result__stat">
                  <span className="import-result__num">{result.skipped}</span>
                  <span className="import-result__label">Skipped (duplicates)</span>
                </div>
              </div>
              <p style={{ margin: 0, fontSize: "0.8rem", color: "var(--secondary)" }}>
                New lines are categorised to this account's Imbalance account — recategorise them from the register.
              </p>
            </>
          ) : (
            <>
              <button
                type="button"
                className="import-dropzone"
                onClick={() => inputRef.current?.click()}
                style={{ width: "100%", cursor: "pointer", background: "transparent" }}
              >
                {file ? <span className="mono">{file.name}</span> : "Click to choose an OFX, QIF, or CSV statement"}
              </button>
              <input
                ref={inputRef}
                type="file"
                accept=".ofx,.qfx,.qif,.csv,text/csv,application/x-ofx,application/qif"
                style={{ display: "none" }}
                onChange={(e) => {
                  setError(null);
                  setPreview(null);
                  setFile(e.target.files?.[0] ?? null);
                }}
              />

              {csv && preview && (
                <div className="csv-map">
                  <label className="csv-map__row">
                    <input type="checkbox" checked={hasHeader} onChange={(e) => setHasHeader(e.target.checked)} />
                    <span>First row is a header</span>
                  </label>

                  <div className="csv-map__grid">
                    <label className="field">
                      <span className="field__label">Date column</span>
                      {colSelect(dateCol, setDateCol)}
                    </label>
                    <label className="field">
                      <span className="field__label">Date format</span>
                      <select value={dateFormat} onChange={(e) => setDateFormat(e.target.value)}>
                        {DATE_FORMATS.map((f) => <option key={f.label} value={f.value}>{f.label}</option>)}
                      </select>
                    </label>
                    <label className="field">
                      <span className="field__label">Amount columns</span>
                      <select value={amountMode} onChange={(e) => setAmountMode(e.target.value as "single" | "debitcredit")}>
                        <option value="single">Single signed column</option>
                        <option value="debitcredit">Separate debit / credit</option>
                      </select>
                    </label>
                    {amountMode === "single" ? (
                      <>
                        <label className="field">
                          <span className="field__label">Amount column</span>
                          {colSelect(amountCol, setAmountCol)}
                        </label>
                        <label className="csv-map__row">
                          <input type="checkbox" checked={invert} onChange={(e) => setInvert(e.target.checked)} />
                          <span>Outflows are positive (invert)</span>
                        </label>
                      </>
                    ) : (
                      <>
                        <label className="field">
                          <span className="field__label">Debit (out) column</span>
                          {colSelect(debitCol, setDebitCol)}
                        </label>
                        <label className="field">
                          <span className="field__label">Credit (in) column</span>
                          {colSelect(creditCol, setCreditCol)}
                        </label>
                      </>
                    )}
                  </div>

                  <div className="field">
                    <span className="field__label">Description columns</span>
                    <div className="csv-map__desc">
                      {columnLabels.map((label, i) => (
                        <label key={i} className="csv-map__chip">
                          <input type="checkbox" checked={descCols.includes(i)} onChange={() => toggleDescCol(i)} />
                          <span>{label}</span>
                        </label>
                      ))}
                    </div>
                  </div>

                  <div className="csv-map__preview">
                    <table className="ledger-table" style={{ fontSize: "0.78rem" }}>
                      <thead>
                        <tr>{columnLabels.map((label, i) => <th key={i}>{label}</th>)}</tr>
                      </thead>
                      <tbody>
                        {dataRows.slice(0, 5).map((row, r) => (
                          <tr key={r}>{columnLabels.map((_, i) => <td key={i}>{row[i] ?? ""}</td>)}</tr>
                        ))}
                      </tbody>
                    </table>
                    <p style={{ margin: "0.4rem 0 0", fontSize: "0.78rem", color: "var(--secondary)" }}>
                      {preview.totalRows}{hasHeader ? " rows (incl. header)" : " rows"} in file.
                    </p>
                  </div>
                </div>
              )}
            </>
          )}
        </div>
        <div className="dialog__footer">
          {result ? (
            <button className="btn btn--primary" onClick={onClose}>Done</button>
          ) : (
            <>
              <button className="btn btn--ghost" onClick={onClose}>Cancel</button>
              <button
                className="btn btn--primary"
                onClick={handleImport}
                disabled={!file || importing || (csv && !preview)}
              >
                {importing ? "Importing…" : "Import"}
              </button>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
