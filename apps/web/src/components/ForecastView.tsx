import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api } from "../lib/api";
import type { Book, CashFlowForecast } from "../lib/types";
import { formatMoney, toFloat } from "../lib/money";

interface Props {
  book: Book;
  onBack?: () => void;
}

const HORIZONS = [3, 6, 12];

function shortMonth(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime())
    ? iso
    : d.toLocaleDateString(undefined, { month: "short" }).toUpperCase();
}
function shortDate(iso: string): string {
  const d = new Date(iso);
  return Number.isNaN(d.getTime())
    ? iso
    : d.toLocaleDateString(undefined, { month: "short", day: "numeric" });
}

// LineChart renders the projected-cash curve as an SVG area + line, starting
// from the current balance through each monthly projection.
function LineChart({ fc }: { fc: CashFlowForecast }) {
  const W = 1000;
  const H = 280;
  const padY = 24;

  // Series: the opening balance, then each month's projected close.
  const series = [
    { label: "Now", value: toFloat(fc.startingCash) },
    ...fc.points.map((p) => ({ label: shortMonth(p.date), value: toFloat(p.projectedCash) })),
  ];
  const values = series.map((s) => s.value);
  let min = Math.min(...values, 0);
  let max = Math.max(...values, 0);
  if (min === max) {
    min -= 1;
    max += 1;
  }
  const span = max - min;
  const x = (i: number) => (i / (series.length - 1)) * W;
  const y = (v: number) => H - padY - ((v - min) / span) * (H - 2 * padY);

  const linePath = series.map((s, i) => `${i === 0 ? "M" : "L"}${x(i)},${y(s.value)}`).join(" ");
  const areaPath = `${linePath} L${W},${H} L0,${H} Z`;
  const zeroY = y(0);

  return (
    <div className="forecast-chart">
      <svg viewBox={`0 0 ${W} ${H}`} preserveAspectRatio="none" className="forecast-chart__svg">
        <defs>
          <linearGradient id="fc-fill" x1="0" x2="0" y1="0" y2="1">
            <stop offset="0%" stopColor="rgba(0,32,69,0.16)" />
            <stop offset="100%" stopColor="rgba(0,32,69,0)" />
          </linearGradient>
        </defs>
        {/* zero baseline (only when it's within view) */}
        {min < 0 && (
          <line x1="0" x2={W} y1={zeroY} y2={zeroY} stroke="var(--error)" strokeDasharray="4 4" strokeWidth="1" />
        )}
        <path d={areaPath} fill="url(#fc-fill)" />
        <path d={linePath} fill="none" stroke="var(--primary)" strokeWidth="3" strokeLinejoin="round" strokeLinecap="round" />
        {series.map((s, i) => (
          <circle key={i} cx={x(i)} cy={y(s.value)} r="4" fill="var(--surface-container-lowest)" stroke="var(--primary)" strokeWidth="2.5" />
        ))}
      </svg>
      <div className="forecast-chart__labels">
        {series.map((s, i) => (
          <span key={i}>{s.label}</span>
        ))}
      </div>
    </div>
  );
}

export function ForecastView({ book, onBack }: Props) {
  const [months, setMonths] = useState(6);

  const q = useQuery({
    queryKey: ["cash-flow-forecast", book.guid, months],
    queryFn: () => api.getCashFlowForecast(book.guid, months),
  });

  return (
    <section className="register report">
      <header className="register__header">
        <div className="register__title">
          {onBack ? (
            <button className="back-link" onClick={onBack}>
              ‹ Reports Center
            </button>
          ) : (
            <div className="eyebrow">Reports</div>
          )}
          <h1>Cash flow forecast</h1>
        </div>
        <div className="report__tabs">
          {HORIZONS.map((m) => (
            <button
              key={m}
              className={`btn btn--sm ${months === m ? "btn--accent" : "btn--ghost"}`}
              onClick={() => setMonths(m)}
            >
              {m} mo
            </button>
          ))}
        </div>
      </header>

      {q.isLoading ? (
        <div className="empty">
          <span className="spinner" />
        </div>
      ) : q.isError ? (
        <div className="empty">Could not load the forecast.</div>
      ) : (
        q.data && (
          <>
            {/* Summary */}
            <div className="forecast-summary">
              <div className="cf-card cf-card--primary">
                <span className="cf-card__label">Projected balance · {months} mo</span>
                <span className="cf-card__value mono">{formatMoney(q.data.endingCash)}</span>
              </div>
              <div className="cf-card">
                <span className="cf-card__label">Net change</span>
                <span className={`cf-card__value mono${q.data.netChange.num < 0 ? " neg" : " pos"}`}>
                  {q.data.netChange.num >= 0 ? "+" : ""}
                  {formatMoney(q.data.netChange)}
                </span>
              </div>
              <div className="cf-card">
                <span className="cf-card__label">Lowest point</span>
                <span className={`cf-card__value mono${q.data.lowestCash.num < 0 ? " neg" : ""}`}>
                  {formatMoney(q.data.lowestCash)}
                </span>
                <span className="forecast-sub">{shortDate(q.data.lowestDate)}</span>
              </div>
            </div>

            {q.data.lowestCash.num < 0 && (
              <div className="forecast-alert">
                ⚠ Projected cash goes negative around {shortDate(q.data.lowestDate)} — review upcoming
                outflows.
              </div>
            )}

            <div className="forecast-grid">
              {/* Chart */}
              <div className="card forecast-chart-card">
                <div className="card__label">Projected net cash position</div>
                <LineChart fc={q.data} />
              </div>

              {/* Upcoming events */}
              <div className="card forecast-events">
                <div className="card__label">Upcoming events</div>
                {q.data.events.length === 0 ? (
                  <div className="empty">No scheduled transactions affect cash in this window.</div>
                ) : (
                  <ul className="event-list">
                    {q.data.events.slice(0, 12).map((e, i) => {
                      const inflow = e.amount.num >= 0;
                      return (
                        <li className="event" key={i}>
                          <span className={`event__icon ${inflow ? "event__icon--in" : "event__icon--out"}`}>
                            {inflow ? "↓" : "↑"}
                          </span>
                          <div className="event__body">
                            <div className="event__row">
                              <span className="event__name">{e.name}</span>
                              <span className={`mono event__amt${inflow ? " pos" : " neg"}`}>
                                {inflow ? "+" : ""}
                                {formatMoney(e.amount)}
                              </span>
                            </div>
                            <span className="event__date">{shortDate(e.date)}</span>
                          </div>
                        </li>
                      );
                    })}
                  </ul>
                )}
              </div>
            </div>

            <p className="report__note">
              Projected from enabled scheduled transactions that touch a cash account. Actual results
              will vary; single-currency only.
            </p>
          </>
        )
      )}
    </section>
  );
}
