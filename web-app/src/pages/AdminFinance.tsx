import { useCallback, useEffect, useState } from 'react';
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

function periodsFor(period: Exclude<FinancePeriod, 'custom'>): number {
  if (period === 'day') return 30;
  if (period === 'year') return 5;
  return 12;
}

function FinanceTrendChart({ report }: { report: FinanceReport }) {
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
    <svg viewBox={`0 0 ${width} ${height}`} width="100%" role="img" aria-label="Finance trend">
      <polyline fill="none" stroke="var(--digital-card-hint)" strokeWidth="1.5" points={grossPts} />
      <polyline fill="none" stroke="#e74c3c" strokeWidth="1.5" points={refundPts} />
      <polyline fill="none" stroke="var(--digital-card-text)" strokeWidth="2" points={netPts} />
    </svg>
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
  const [error, setError] = useState<string | null>(null);
  const [sessionExpired, setSessionExpired] = useState(false);
  const [isAdmin, setIsAdmin] = useState<boolean | null>(null);
  const [expanded, setExpanded] = useState<Record<string, boolean>>({});

  const load = useCallback(async () => {
    if (!initData) {
      setLoading(false);
      return;
    }

    // Custom range needs both dates before querying (query builder rejects empty/invalid).
    if (period === 'custom' && (!from || !to)) {
      setLoading(false);
      setReport(null);
      setError(null);
      return;
    }

    setLoading(true);
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
        setError(e instanceof APIError ? e.body || e.message : 'Failed to load finance');
      }
    } finally {
      setLoading(false);
    }
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
      setError('Select a custom date range before export');
      return;
    }

    playClick();
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
        setError('Export failed');
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
      setError('Export failed');
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

  if (loading || isAdmin === null) {
    return <LoadingScreen />;
  }

  if (isAdmin === false) {
    return <div role="alert">{t('finance_admin_required')}</div>;
  }

  if (error) {
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
    <div style={{ padding: 16, maxWidth: 480, margin: '0 auto' }}>
      <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 8 }}>
        <h1 style={{ margin: 0 }}>{t('finance_title')}</h1>
        <button type="button" onClick={() => void onExport()}>
          {t('finance_export_csv')}
        </button>
      </div>

      <div style={{ fontSize: 12, color: 'var(--digital-card-hint)' }}>
        {report?.timezone ?? 'Asia/Yangon'} · {report?.range_start}
        {report?.in_progress ? ` · ${t('finance_in_progress')}` : ''}
      </div>

      <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginTop: 12 }}>
        {PERIOD_TABS.map(([value, label]) => (
          <button
            key={value}
            type="button"
            aria-pressed={period === value}
            onClick={() => {
              playClick();
              setPeriod(value);
            }}
          >
            {label}
          </button>
        ))}
      </div>

      {period === 'custom' && (
        <div style={{ display: 'flex', gap: 8, marginTop: 8 }}>
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
          <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 8, marginTop: 16 }}>
            <div className="digital-card" style={{ padding: 12, gridColumn: '1 / -1' }}>
              <div>{t('finance_net_income')}</div>
              <strong>{formatMoneyMMK(report.current.net_service_revenue)}</strong>
              {report.in_progress && <span> · {t('finance_in_progress')}</span>}
              {report.delta && <div>{formatDelta(report.delta.net_service_revenue)}</div>}
            </div>
            <div className="digital-card" style={{ padding: 12 }}>
              <div>{t('finance_gross')}</div>
              <strong>{formatMoneyMMK(report.current.gross_service_revenue)}</strong>
            </div>
            <div className="digital-card" style={{ padding: 12 }}>
              <div>{t('finance_refunds')}</div>
              <strong>{formatMoneyMMK(report.current.refunds)}</strong>
            </div>
            <div className="digital-card" style={{ padding: 12 }}>
              <div>{t('finance_cash')}</div>
              <strong>{formatMoneyMMK(report.current.cash_collected)}</strong>
            </div>
          </div>

          <div style={{ marginTop: 16 }}>
            <FinanceTrendChart report={report} />
          </div>

          <ul style={{ marginTop: 16, paddingLeft: 18 }}>
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

          <table style={{ width: '100%', marginTop: 16, fontSize: 13 }}>
            <thead>
              <tr>
                <th align="left">Period</th>
                <th align="right">Net</th>
                <th align="right">Gross</th>
                <th align="right">Refunds</th>
              </tr>
            </thead>
            <tbody>
              {report.trend.map((b) => (
                <tr key={b.period_start}>
                  <td>
                    <button
                      type="button"
                      onClick={() =>
                        setExpanded((prev) => ({
                          ...prev,
                          [b.period_start]: !prev[b.period_start],
                        }))
                      }
                    >
                      {b.period_start}
                      {b.in_progress ? ' *' : ''}
                    </button>
                    {expanded[b.period_start] && (
                      <div>
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
                  <td align="right">{formatMoneyMMK(b.metrics.net_service_revenue)}</td>
                  <td align="right">{formatMoneyMMK(b.metrics.gross_service_revenue)}</td>
                  <td align="right">{formatMoneyMMK(b.metrics.refunds)}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </>
      )}

      {isEmpty && <p>{t('finance_empty')}</p>}
    </div>
  );
}
