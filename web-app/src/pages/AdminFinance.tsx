import { useCallback, useEffect, useState, type CSSProperties } from 'react';
import { useNavigate } from 'react-router-dom';
import { LoadingScreen } from '../components/LoadingScreen';
import { SessionExpiredScreen } from '../components/SessionExpiredScreen';
import {
  clearTelegramSession,
  fetchJSONWithTelegramAuth,
  fetchUserScopedJSONWithTelegramAuth,
  fetchWithTelegramAuth,
} from '../lib/auth';
import { APIError, isAPIStatus } from '../lib/http';
import {
  buildRevenueExportQuery,
  buildRevenueQuery,
  buildTrendPolylinePoints,
  formatDelta,
  formatMoneyMMK,
  type FinancePeriod,
  type FinanceReport,
} from '../lib/finance';
import { useLanguage } from '../lib/LanguageContext';
import { UserData } from '../lib/types';
import { useTelegram } from '../lib/twa';
import { useMXBrownSound } from '../lib/useMXBrownSound';

const PERIOD_TABS = [
  ['day', 'Daily'],
  ['week', 'Weekly'],
  ['month', 'Monthly'],
  ['year', 'Yearly'],
  ['custom', 'Custom'],
] as const;

const cardChrome: CSSProperties = {
  padding: 14,
  borderRadius: 16,
  background: 'var(--digital-card-bg, rgba(255,255,255,0.06))',
  backdropFilter: 'blur(12px)',
  boxShadow: '0 4px 16px rgba(0,0,0,0.12)',
  border: '1px solid var(--digital-card-border, rgba(255,255,255,0.08))',
};

function periodsFor(period: Exclude<FinancePeriod, 'custom'>): number {
  if (period === 'day') return 30;
  if (period === 'year') return 5;
  return 12;
}

function trendIsEmpty(report: FinanceReport): boolean {
  if (report.trend.length === 0) return true;
  return report.trend.every(
    (b) =>
      b.metrics.gross_service_revenue === 0 &&
      b.metrics.refunds === 0 &&
      b.metrics.net_service_revenue === 0 &&
      b.metrics.successful_orders === 0,
  );
}

function FinanceTrendChart({ report }: { report: FinanceReport }) {
  const { t } = useLanguage();
  if (trendIsEmpty(report)) {
    return (
      <div
        role="img"
        aria-label="Finance trend empty"
        style={{
          ...cardChrome,
          minHeight: 120,
          display: 'flex',
          alignItems: 'center',
          justifyContent: 'center',
          color: 'var(--digital-card-hint)',
          fontSize: 13,
          textAlign: 'center',
        }}
      >
        {t('finance_trend_empty')}
      </div>
    );
  }

  const width = 320;
  const height = 140;
  const pad = 16;
  const gross = report.trend.map((b) => b.metrics.gross_service_revenue);
  const refunds = report.trend.map((b) => b.metrics.refunds);
  const net = report.trend.map((b) => b.metrics.net_service_revenue);
  const grossPts = buildTrendPolylinePoints(gross, width, height, pad);
  const refundPts = buildTrendPolylinePoints(refunds, width, height, pad);
  const netPts = buildTrendPolylinePoints(net, width, height, pad);

  return (
    <div style={{ ...cardChrome, padding: 12 }}>
      <svg viewBox={`0 0 ${width} ${height}`} width="100%" role="img" aria-label="Finance trend">
        <polyline fill="none" stroke="var(--digital-card-hint)" strokeWidth="1.5" points={grossPts} />
        <polyline fill="none" stroke="#e74c3c" strokeWidth="1.5" points={refundPts} />
        <polyline fill="none" stroke="var(--digital-card-text)" strokeWidth="2" points={netPts} />
      </svg>
    </div>
  );
}

export function AdminFinance() {
  const { t } = useLanguage();
  const { playClick } = useMXBrownSound();
  const navigate = useNavigate();
  const { tg, initData, close } = useTelegram();
  const [period, setPeriod] = useState<FinancePeriod>('day');
  const [from, setFrom] = useState('');
  const [to, setTo] = useState('');
  const [report, setReport] = useState<FinanceReport | null>(null);
  const [loading, setLoading] = useState(true);
  const [softLoading, setSoftLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [exportError, setExportError] = useState<string | null>(null);
  const [sessionExpired, setSessionExpired] = useState(false);
  const [isAdmin, setIsAdmin] = useState<boolean | null>(null);
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});

  const load = useCallback(async () => {
    if (!initData) {
      setLoading(false);
      setSoftLoading(false);
      return;
    }

    // Custom range needs both dates before querying (query builder rejects empty/invalid).
    if (period === 'custom' && (!from || !to)) {
      setLoading(false);
      setSoftLoading(false);
      setError(null);
      return;
    }

    const hasReport = report != null;
    if (hasReport) {
      setSoftLoading(true);
    } else {
      setLoading(true);
    }
    setError(null);
    try {
      const me = await fetchUserScopedJSONWithTelegramAuth<UserData>(
        '/api/me',
        initData,
        tg?.initDataUnsafe?.user?.id,
      );
      if (!me.is_admin) {
        setIsAdmin(false);
        setLoading(false);
        setSoftLoading(false);
        return;
      }
      setIsAdmin(true);

      const url =
        period === 'custom'
          ? buildRevenueQuery({ period, from, to })
          : buildRevenueQuery({ period, periods: periodsFor(period) });
      const data = await fetchJSONWithTelegramAuth<FinanceReport>(url, initData);
      setReport(data);
    } catch (e) {
      if (isAPIStatus(e, 401)) {
        clearTelegramSession();
        setSessionExpired(true);
      } else {
        // Keep last successful report visible; only set page error when nothing to show.
        const msg = e instanceof APIError ? e.body || e.message : 'Failed to load finance';
        if (!hasReport) {
          setError(msg);
        } else {
          setExportError(msg);
        }
      }
    } finally {
      setLoading(false);
      setSoftLoading(false);
    }
    // report intentionally omitted from deps: used only as "has prior data" flag for soft loading.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [period, from, to, initData, tg]);

  useEffect(() => {
    void load();
  }, [load]);

  useEffect(() => {
    if (!tg?.BackButton) return;
    tg.BackButton.show();
    const handler = () => navigate('/');
    tg.BackButton.onClick(handler);
    return () => {
      tg.BackButton.offClick(handler);
      tg.BackButton.hide();
    };
  }, [navigate, tg]);

  const onExport = async () => {
    if (!initData) return;
    if (period === 'custom' && (!from || !to)) {
      setExportError('Select a custom date range before export');
      return;
    }

    playClick();
    setExportError(null);
    try {
      const url =
        period === 'custom'
          ? buildRevenueExportQuery({ period, from, to })
          : buildRevenueExportQuery({ period, periods: periodsFor(period) });
      const res = await fetchWithTelegramAuth(url, initData);
      if (res.status === 401) {
        clearTelegramSession();
        setSessionExpired(true);
        return;
      }
      if (!res.ok) {
        setExportError(t('finance_export_failed'));
        return;
      }
      const blob = await res.blob();
      const objectUrl = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = objectUrl;
      a.download = 'finance-report.csv';
      a.click();
      URL.revokeObjectURL(objectUrl);
    } catch {
      setExportError(t('finance_export_failed'));
    }
  };

  if (sessionExpired) {
    return (
      <SessionExpiredScreen
        title={t('session_expired_title')}
        message={t('session_expired_desc')}
        reloadLabel={t('session_expired_reload')}
        closeLabel={t('session_expired_close')}
        onReload={() => {
          window.location.reload();
        }}
        onClose={() => {
          close();
        }}
      />
    );
  }

  // Full-page loading only on first load (no report yet).
  if ((loading && report == null) || isAdmin === null) {
    return <LoadingScreen />;
  }

  if (isAdmin === false) {
    return <div role="alert">{t('finance_admin_required')}</div>;
  }

  // First-load error with no report: full error state.
  if (error && report == null) {
    return (
      <div role="alert">
        {error}
        <button type="button" onClick={() => void load()}>
          Retry
        </button>
      </div>
    );
  }

  const isEmpty =
    report != null &&
    report.current.successful_orders === 0 &&
    report.current.gross_service_revenue === 0;

  return (
    <div
      style={{
        padding: 16,
        maxWidth: 480,
        margin: '0 auto',
        opacity: softLoading ? 0.72 : 1,
        transition: 'opacity 0.15s ease',
      }}
      data-soft-loading={softLoading ? 'true' : 'false'}
    >
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 8 }}>
        <h1 style={{ margin: 0, fontSize: 22 }}>{t('finance_title')}</h1>
        <button
          type="button"
          onClick={() => void onExport()}
          style={{
            padding: '8px 12px',
            borderRadius: 12,
            border: '1px solid var(--digital-card-border, rgba(255,255,255,0.12))',
            background: 'var(--digital-card-inner-bg)',
            color: 'var(--digital-card-text)',
            cursor: 'pointer',
          }}
        >
          {t('finance_export_csv')}
        </button>
      </div>

      <div style={{ fontSize: 12, color: 'var(--digital-card-hint)', marginTop: 4 }}>
        {report?.timezone ?? 'Asia/Yangon'} · {report?.range_start}
        {report?.range_end && report.range_end !== report.range_start ? ` → ${report.range_end}` : ''}
        {report?.in_progress ? ` · ${t('finance_in_progress')}` : ''}
        {softLoading ? ' · …' : ''}
      </div>

      {exportError && (
        <div
          role="alert"
          data-testid="export-error"
          style={{
            marginTop: 10,
            padding: '10px 12px',
            borderRadius: 12,
            background: 'rgba(231, 76, 60, 0.12)',
            color: 'var(--digital-card-text)',
            fontSize: 13,
          }}
        >
          {exportError}
        </div>
      )}

      <div style={{ display: 'flex', gap: 6, flexWrap: 'wrap', marginTop: 12 }}>
        {PERIOD_TABS.map(([value, label]) => (
          <button
            key={value}
            type="button"
            aria-pressed={period === value}
            onClick={() => {
              playClick();
              setPeriod(value);
            }}
            style={{
              padding: '8px 12px',
              borderRadius: 20,
              border: period === value
                ? '1px solid var(--digital-card-text)'
                : '1px solid var(--digital-card-border, rgba(255,255,255,0.12))',
              background: period === value
                ? 'var(--digital-card-inner-bg)'
                : 'transparent',
              color: 'var(--digital-card-text)',
              cursor: 'pointer',
              fontSize: 13,
            }}
          >
            {label}
          </button>
        ))}
      </div>

      {period === 'custom' && (
        <div style={{ display: 'flex', gap: 8, marginTop: 10 }}>
          <input
            aria-label="From date"
            type="date"
            value={from}
            onChange={(e) => setFrom(e.target.value)}
          />
          <input
            aria-label="To date"
            type="date"
            value={to}
            onChange={(e) => setTo(e.target.value)}
          />
        </div>
      )}

      {report && (
        <>
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10, marginTop: 14 }}>
            <div className="digital-card" style={{ ...cardChrome, gridColumn: '1 / -1' }} data-testid="headline-net">
              <div style={{ fontSize: 13, color: 'var(--digital-card-hint)' }}>{t('finance_net_income')}</div>
              <strong style={{ fontSize: 22 }}>{formatMoneyMMK(report.current.net_service_revenue)}</strong>
              {report.in_progress && (
                <span style={{ marginLeft: 6, fontSize: 12, color: 'var(--digital-card-hint)' }}>
                  · {t('finance_in_progress')}
                </span>
              )}
              {report.delta && (
                <div style={{ fontSize: 12, marginTop: 4 }}>{formatDelta(report.delta.net_service_revenue)}</div>
              )}
            </div>
            <div className="digital-card" style={cardChrome}>
              <div style={{ fontSize: 13, color: 'var(--digital-card-hint)' }}>{t('finance_gross')}</div>
              <strong>{formatMoneyMMK(report.current.gross_service_revenue)}</strong>
            </div>
            <div className="digital-card" style={cardChrome}>
              <div style={{ fontSize: 13, color: 'var(--digital-card-hint)' }}>{t('finance_refunds')}</div>
              <strong>{formatMoneyMMK(report.current.refunds)}</strong>
            </div>
            <div className="digital-card" style={{ ...cardChrome, gridColumn: '1 / -1' }}>
              <div style={{ fontSize: 13, color: 'var(--digital-card-hint)' }}>{t('finance_cash')}</div>
              <strong>{formatMoneyMMK(report.current.cash_collected)}</strong>
            </div>
          </div>

          <div style={{ marginTop: 14 }}>
            <FinanceTrendChart report={report} />
          </div>

          <ul
            style={{
              marginTop: 14,
              padding: 14,
              listStyle: 'none',
              ...cardChrome,
              fontSize: 13,
              display: 'grid',
              gap: 6,
            }}
          >
            <li>
              {t('finance_wallet_topups')}: {formatMoneyMMK(report.current.wallet_topups)}
            </li>
            <li>
              {t('finance_wallet_spend')}: {formatMoneyMMK(report.current.wallet_spend)}
            </li>
            <li>
              {t('finance_new_subs')}: {report.current.new_subscriptions}
            </li>
            <li>
              {t('finance_extensions')}: {report.current.extensions}
            </li>
            <li>
              {t('finance_orders')}: {report.current.successful_orders}
            </li>
            <li>
              {t('finance_customers')}: {report.current.unique_customers}
            </li>
            <li>
              {t('finance_aov')}: {formatMoneyMMK(report.current.average_order_value)}
            </li>
          </ul>

          <div style={{ ...cardChrome, marginTop: 14, padding: 10, overflowX: 'auto' }}>
            <table style={{ width: '100%', fontSize: 13, borderCollapse: 'collapse' }}>
              <thead>
                <tr>
                  <th align="left" style={{ padding: '6px 4px' }}>Period</th>
                  <th align="right" style={{ padding: '6px 4px' }}>Net</th>
                  <th align="right" style={{ padding: '6px 4px' }}>Gross</th>
                  <th align="right" style={{ padding: '6px 4px' }}>Refunds</th>
                </tr>
              </thead>
              <tbody>
                {report.trend.map((b) => (
                  <tr key={b.period_start}>
                    <td style={{ padding: '6px 4px', verticalAlign: 'top' }}>
                      <button
                        type="button"
                        onClick={() =>
                          setExpanded((prev) => ({
                            ...prev,
                            [b.period_start]: !prev[b.period_start],
                          }))
                        }
                        style={{
                          background: 'transparent',
                          border: 'none',
                          color: 'var(--digital-card-text)',
                          cursor: 'pointer',
                          padding: 0,
                          textAlign: 'left',
                        }}
                      >
                        {b.period_start}
                        {b.in_progress ? ' *' : ''}
                      </button>
                      {expanded[b.period_start] && (
                        <div style={{ marginTop: 4, fontSize: 12, color: 'var(--digital-card-hint)' }}>
                          {b.categories.map((c) => (
                            <div key={c.category}>
                              {c.category}: {c.orders} / {formatMoneyMMK(c.amount)}
                            </div>
                          ))}
                          {b.methods.map((m) => (
                            <div key={m.method}>
                              {m.method}: {formatMoneyMMK(m.service_revenue)}
                            </div>
                          ))}
                        </div>
                      )}
                    </td>
                    <td align="right" style={{ padding: '6px 4px' }}>{formatMoneyMMK(b.metrics.net_service_revenue)}</td>
                    <td align="right" style={{ padding: '6px 4px' }}>{formatMoneyMMK(b.metrics.gross_service_revenue)}</td>
                    <td align="right" style={{ padding: '6px 4px' }}>{formatMoneyMMK(b.metrics.refunds)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </>
      )}

      {isEmpty && <p style={{ marginTop: 12, color: 'var(--digital-card-hint)' }}>{t('finance_empty')}</p>}
    </div>
  );
}
