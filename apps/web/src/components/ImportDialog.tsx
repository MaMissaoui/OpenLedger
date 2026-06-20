import { useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";

import { api } from "../lib/api";
import type { ImportResult } from "../lib/types";

// ImportDialog uploads a GnuCash file (SQLite or XML, optionally gzipped) and
// imports it as a new book. On success it shows the object counts; the caller
// closes the dialog. Because books are reference for the whole shell, a
// successful import invalidates the cached book list.
export default function ImportDialog({ onClose }: { onClose: () => void }) {
  const queryClient = useQueryClient();
  const inputRef = useRef<HTMLInputElement>(null);
  const [file, setFile] = useState<File | null>(null);
  const [importing, setImporting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [result, setResult] = useState<ImportResult | null>(null);

  async function handleImport() {
    if (!file) return;
    setError(null);
    setImporting(true);
    try {
      const res = await api.importGnuCash(file);
      setResult(res);
      void queryClient.invalidateQueries({ queryKey: ["books"] });
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
          <h2>Import GnuCash File</h2>
          <button className="dialog__close" onClick={onClose} aria-label="Close">×</button>
        </div>
        <div className="dialog__body">
          {error && <p className="error" style={{ margin: 0 }}>{error}</p>}

          {result ? (
            <>
              <p style={{ margin: 0 }}>Imported a new book.</p>
              <div className="import-result">
                <div className="import-result__stat">
                  <span className="import-result__num">{result.accounts}</span>
                  <span className="import-result__label">Accounts</span>
                </div>
                <div className="import-result__stat">
                  <span className="import-result__num">{result.transactions}</span>
                  <span className="import-result__label">Transactions</span>
                </div>
                <div className="import-result__stat">
                  <span className="import-result__num">{result.commodities}</span>
                  <span className="import-result__label">Commodities</span>
                </div>
              </div>
              <p style={{ margin: 0, fontSize: "0.8rem", color: "var(--secondary)" }} className="mono">
                book {result.bookGuid.slice(0, 8)}…
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
                {file ? (
                  <span className="mono">{file.name}</span>
                ) : (
                  "Click to choose a GnuCash file (.gnucash, SQLite, or XML — gzip OK)"
                )}
              </button>
              <input
                ref={inputRef}
                type="file"
                accept=".gnucash,.sqlite,.sqlite3,.db,.xml,.gz,application/x-sqlite3,application/xml"
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
