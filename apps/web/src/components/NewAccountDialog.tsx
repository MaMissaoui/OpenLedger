import { useState, type FormEvent } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { api, ApiError } from "../lib/api";
import type { Account } from "../lib/types";
import { ACCOUNT_TYPES } from "../lib/types";

interface Props {
  bookGuid: string;
  accounts: Account[];
  onClose: () => void;
}

// NewAccountDialog adds an account to the book. The commodity is inherited from
// the existing chart (single-currency in this slice); the parent defaults to a
// chosen placeholder group or the book root.
export function NewAccountDialog({ bookGuid, accounts, onClose }: Props) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const [name, setName] = useState("");
  const [type, setType] = useState<string>("EXPENSE");
  const [parentGuid, setParentGuid] = useState("");
  const [code, setCode] = useState("");
  const [error, setError] = useState("");

  const groups = accounts.filter((a) => a.placeholder);
  const commodityGuid = accounts.find((a) => a.commodityGuid)?.commodityGuid ?? "";

  const create = useMutation({
    mutationFn: () =>
      api.createAccount({
        bookGuid,
        name: name.trim(),
        type,
        commodityGuid,
        parentGuid: parentGuid || undefined,
        code: code.trim() || undefined,
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["accounts", bookGuid] });
      onClose();
    },
    onError: (err) => setError(err instanceof ApiError ? err.message : t("accounts.createAccount") + " failed"),
  });

  function submit(e: FormEvent) {
    e.preventDefault();
    setError("");
    if (name.trim()) create.mutate();
  }

  return (
    <div className="dialog-backdrop" onMouseDown={onClose}>
      <div className="dialog" onMouseDown={(e) => e.stopPropagation()}>
        <h2>{t("accounts.newAccount")}</h2>
        <p className="sub">{t("accounts.addToChart")}</p>

        <form className="dialog__grid" onSubmit={submit}>
          <div className="field">
            <label htmlFor="ac-name">{t("common.name")}</label>
            <input
              id="ac-name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={t("accounts.namePlaceholder")}
              autoFocus
            />
          </div>

          <div className="dialog__row">
            <div className="field">
              <label htmlFor="ac-type">{t("common.type")}</label>
              <select id="ac-type" value={type} onChange={(e) => setType(e.target.value)}>
                {ACCOUNT_TYPES.map((acType) => (
                  <option key={acType} value={acType}>
                    {acType}
                  </option>
                ))}
              </select>
            </div>
            <div className="field">
              <label htmlFor="ac-code">{t("accounts.codeOptional")}</label>
              <input
                id="ac-code"
                className="mono"
                value={code}
                onChange={(e) => setCode(e.target.value)}
                placeholder={t("accounts.codePlaceholder")}
              />
            </div>
          </div>

          <div className="field">
            <label htmlFor="ac-parent">{t("accounts.parent")}</label>
            <select id="ac-parent" value={parentGuid} onChange={(e) => setParentGuid(e.target.value)}>
              <option value="">{t("accounts.topLevel")}</option>
              {groups.map((g) => (
                <option key={g.guid} value={g.guid}>
                  {g.name}
                </option>
              ))}
            </select>
          </div>

          <div className="error-note">{error}</div>

          <div className="dialog__actions">
            <button type="button" className="btn btn--ghost" onClick={onClose}>
              {t("common.cancel")}
            </button>
            <button type="submit" className="btn btn--accent" disabled={!name.trim() || create.isPending}>
              {create.isPending ? <span className="spinner" /> : t("accounts.createAccount")}
            </button>
          </div>
        </form>
      </div>
    </div>
  );
}
