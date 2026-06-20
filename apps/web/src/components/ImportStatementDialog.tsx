import { useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";

import { api } from "../lib/api";
import type { Account, BankImportResult } from "../lib/types";

// ImportStatementDialog uploads an OFX/QIF bank statement into an account. The
// server sniffs the format and posts each line against the book's Imbalance
// account, skipping duplicates. On success it shows the imported/skipped counts;
// the register and balances are refreshed.
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

  async function handleImport() {
    if (!file) return;
    setError(null);
    setImporting(true);
    try {
      const res = await api.importBankStatement(account.guid, file);
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

  return (
    <div className="dialog-overlay" onClick={onClose}>
      <div className="dialog" onClick={(e) => e.stopPropagation()}>
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
                {file ? <span className="mono">{file.name}</span> : "Click to choose an OFX or QIF statement"}
              </button>
              <input
                ref={inputRef}
                type="file"
                accept=".ofx,.qfx,.qif,application/x-ofx,application/qif"
                style={{ display: "none" }}
                onChange={(e) => {
                  setError(null);
                  setFile(e.target.files?.[0] ?? null);
                }}
              />
            </>
          )}
        </div>
        <div className="dialog__footer">
          {result ? (
            <button className="btn btn--primary" onClick={onClose}>Done</button>
          ) : (
            <>
              <button className="btn btn--ghost" onClick={onClose}>Cancel</button>
              <button className="btn btn--primary" onClick={handleImport} disabled={!file || importing}>
                {importing ? "Importing…" : "Import"}
              </button>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
