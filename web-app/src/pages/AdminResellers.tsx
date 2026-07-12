import { FormEvent, useCallback, useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { ErrorScreen } from '../components/ErrorScreen';
import { LoadingScreen } from '../components/LoadingScreen';
import { SessionExpiredScreen } from '../components/SessionExpiredScreen';
import {
    clearTelegramSession,
    fetchJSONWithTelegramAuth,
    fetchUserScopedJSONWithTelegramAuth,
    fetchWithTelegramAuth,
} from '../lib/auth';
import { APIError, isAPIStatus } from '../lib/http';
import { useLanguage } from '../lib/LanguageContext';
import {
    AdminReseller,
    ResellerLedgerEntry,
    ResellerSettlementResponse,
    UserData,
} from '../lib/types';
import { useMXBrownSound } from '../lib/useMXBrownSound';
import { createIdempotencyKey, useTelegram } from '../lib/twa';

function formatMoney(value: number): string {
    return (value || 0).toLocaleString();
}

// direction increase → +amount (owed up / red)
// direction decrease → -amount (owed down / green)
function resolveLedgerSignedAmount(entry: ResellerLedgerEntry): number {
    const dir = entry.direction.toLowerCase();
    if (dir === 'decrease') return -Math.abs(entry.amount);
    if (dir === 'increase') return Math.abs(entry.amount);
    const t = entry.entry_type.toLowerCase();
    if (t === 'settlement') return -Math.abs(entry.amount);
    return Math.abs(entry.amount);
}

function formatLedgerAmount(entry: ResellerLedgerEntry): string {
    const amount = resolveLedgerSignedAmount(entry);
    const prefix = amount > 0 ? '+' : '';
    return `${prefix}${formatMoney(amount)}`;
}

function ledgerAmountColor(entry: ResellerLedgerEntry): string {
    const amount = resolveLedgerSignedAmount(entry);
    if (amount > 0) return '#ff3b30';
    if (amount < 0) return '#34c759';
    return 'var(--text-color)';
}

export function AdminResellers() {
    const { tg, initData, close } = useTelegram();
    const { t } = useLanguage();
    const navigate = useNavigate();
    const { playClick } = useMXBrownSound();

    const [resellers, setResellers] = useState<AdminReseller[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [authExpired, setAuthExpired] = useState(false);
    const [accessDenied, setAccessDenied] = useState(false);
    const [telegramId, setTelegramId] = useState('');
    const [submitting, setSubmitting] = useState(false);
    const [actionError, setActionError] = useState<string | null>(null);
    const [actionSuccess, setActionSuccess] = useState<string | null>(null);

    // Per-reseller credit / settlement / ledger UI state
    const [expandedId, setExpandedId] = useState<number | null>(null);
    const [creditLimitDrafts, setCreditLimitDrafts] = useState<Record<number, string>>({});
    const [settlementAmounts, setSettlementAmounts] = useState<Record<number, string>>({});
    const [settlementNotes, setSettlementNotes] = useState<Record<number, string>>({});
    const [ledgerById, setLedgerById] = useState<Record<number, ResellerLedgerEntry[]>>({});
    const [ledgerLoadingId, setLedgerLoadingId] = useState<number | null>(null);
    const [ledgerOpenId, setLedgerOpenId] = useState<number | null>(null);
    const [rowBusyId, setRowBusyId] = useState<number | null>(null);

    const handleBack = useCallback(() => {
        navigate('/');
    }, [navigate]);

    const loadResellers = useCallback(async () => {
        if (!initData) {
            setLoading(false);
            return;
        }

        setLoading(true);
        setError(null);
        setAuthExpired(false);
        setAccessDenied(false);

        try {
            const meData = await fetchUserScopedJSONWithTelegramAuth<UserData>(
                '/api/me',
                initData,
                tg?.initDataUnsafe?.user?.id,
            );
            if (!meData.is_admin) {
                setAccessDenied(true);
                return;
            }

            const list = await fetchJSONWithTelegramAuth<AdminReseller[]>('/api/admin/resellers', initData);
            const rows = Array.isArray(list) ? list : [];
            setResellers(rows);
            setCreditLimitDrafts((prev) => {
                const next = { ...prev };
                for (const row of rows) {
                    if (next[row.telegram_id] === undefined) {
                        next[row.telegram_id] = String(row.credit_limit ?? 0);
                    }
                }
                return next;
            });
        } catch (err) {
            if (isAPIStatus(err, 401)) {
                clearTelegramSession();
                setAuthExpired(true);
                return;
            }
            if (isAPIStatus(err, 403)) {
                setAccessDenied(true);
                return;
            }
            if (err instanceof APIError && err.body) {
                setError(err.body);
                return;
            }
            setError(err instanceof Error ? err.message : t('admin_resellers_load_error'));
        } finally {
            setLoading(false);
        }
    }, [initData, t, tg]);

    useEffect(() => {
        if (!tg) return;
        tg.BackButton.show();
        tg.BackButton.onClick(handleBack);
        return () => tg.BackButton.offClick(handleBack);
    }, [handleBack, tg]);

    useEffect(() => {
        void loadResellers();
    }, [loadResellers]);

    const refreshListAndMaybeLedger = async (telegramIdNum?: number) => {
        if (!initData) return;
        try {
            const list = await fetchJSONWithTelegramAuth<AdminReseller[]>('/api/admin/resellers', initData);
            const rows = Array.isArray(list) ? list : [];
            setResellers(rows);
            setCreditLimitDrafts((prev) => {
                const next = { ...prev };
                for (const row of rows) {
                    next[row.telegram_id] = String(row.credit_limit ?? 0);
                }
                return next;
            });
            if (telegramIdNum != null && ledgerOpenId === telegramIdNum) {
                const ledger = await fetchJSONWithTelegramAuth<ResellerLedgerEntry[]>(
                    `/api/admin/customers/${encodeURIComponent(String(telegramIdNum))}/ledger`,
                    initData,
                );
                setLedgerById((prev) => ({
                    ...prev,
                    [telegramIdNum]: Array.isArray(ledger) ? ledger : [],
                }));
            }
        } catch {
            // Keep prior UI; action already succeeded.
        }
    };

    const toggleReseller = async (isReseller: boolean) => {
        if (!initData || submitting) return;

        const trimmed = telegramId.trim();
        if (!/^\d+$/.test(trimmed) || Number(trimmed) <= 0) {
            setActionError(t('admin_resellers_validation_telegram_id'));
            setActionSuccess(null);
            return;
        }

        setSubmitting(true);
        setActionError(null);
        setActionSuccess(null);

        try {
            const response = await fetchWithTelegramAuth(
                `/api/admin/customers/${encodeURIComponent(trimmed)}/reseller`,
                initData,
                {
                    method: 'PATCH',
                    headers: {
                        'Content-Type': 'application/json',
                    },
                    body: JSON.stringify({ is_reseller: isReseller }),
                },
            );

            if (response.status === 401) {
                clearTelegramSession();
                setAuthExpired(true);
                return;
            }
            if (response.status === 403) {
                setAccessDenied(true);
                return;
            }
            if (!response.ok) {
                const message = await response.text();
                setActionError(message || t('admin_resellers_toggle_error'));
                return;
            }

            const updated = await response.json() as AdminReseller;
            setResellers((prev) => {
                if (updated.is_reseller) {
                    const without = prev.filter((item) => item.telegram_id !== updated.telegram_id);
                    return [...without, updated].sort((a, b) => a.telegram_id - b.telegram_id);
                }
                return prev.filter((item) => item.telegram_id !== updated.telegram_id);
            });
            if (updated.is_reseller) {
                setCreditLimitDrafts((prev) => ({
                    ...prev,
                    [updated.telegram_id]: String(updated.credit_limit ?? 0),
                }));
            }
            setTelegramId('');
            setActionSuccess(
                updated.is_reseller
                    ? t('admin_resellers_enable_success')
                    : t('admin_resellers_disable_success'),
            );
        } catch {
            setActionError(t('admin_resellers_toggle_error'));
        } finally {
            setSubmitting(false);
        }
    };

    const handleEnable = (event: FormEvent<HTMLFormElement>) => {
        event.preventDefault();
        playClick();
        void toggleReseller(true);
    };

    const setCreditLimit = async (telegramIdNum: number) => {
        if (!initData || rowBusyId != null) return;

        const raw = (creditLimitDrafts[telegramIdNum] ?? '').trim();
        const limit = Number(raw);
        if (!Number.isFinite(limit) || limit < 0) {
            setActionError(t('admin_resellers_validation_credit_limit'));
            setActionSuccess(null);
            return;
        }

        setRowBusyId(telegramIdNum);
        setActionError(null);
        setActionSuccess(null);

        try {
            const response = await fetchWithTelegramAuth(
                `/api/admin/customers/${encodeURIComponent(String(telegramIdNum))}/credit`,
                initData,
                {
                    method: 'PATCH',
                    headers: {
                        'Content-Type': 'application/json',
                    },
                    body: JSON.stringify({ credit_limit: limit }),
                },
            );

            if (response.status === 401) {
                clearTelegramSession();
                setAuthExpired(true);
                return;
            }
            if (response.status === 403) {
                setAccessDenied(true);
                return;
            }
            if (!response.ok) {
                const message = await response.text();
                setActionError(message || t('admin_resellers_credit_error'));
                return;
            }

            const updated = await response.json() as AdminReseller;
            setResellers((prev) =>
                prev.map((item) =>
                    item.telegram_id === updated.telegram_id ? { ...item, ...updated } : item,
                ),
            );
            setCreditLimitDrafts((prev) => ({
                ...prev,
                [updated.telegram_id]: String(updated.credit_limit ?? 0),
            }));
            setActionSuccess(t('admin_resellers_credit_success'));
            await refreshListAndMaybeLedger(telegramIdNum);
        } catch {
            setActionError(t('admin_resellers_credit_error'));
        } finally {
            setRowBusyId(null);
        }
    };

    const recordSettlement = async (telegramIdNum: number) => {
        if (!initData || rowBusyId != null) return;

        const amount = Number((settlementAmounts[telegramIdNum] ?? '').trim());
        if (!Number.isFinite(amount) || amount <= 0) {
            setActionError(t('admin_resellers_validation_settlement_amount'));
            setActionSuccess(null);
            return;
        }

        const note = (settlementNotes[telegramIdNum] ?? '').trim();
        const idempotencyKey = createIdempotencyKey();

        setRowBusyId(telegramIdNum);
        setActionError(null);
        setActionSuccess(null);

        try {
            // Money-safety: admin offline AR only — no wallet APIs, no payment_method wallet.
            const result = await fetchJSONWithTelegramAuth<ResellerSettlementResponse>(
                `/api/admin/customers/${encodeURIComponent(String(telegramIdNum))}/settlements`,
                initData,
                {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                        'Idempotency-Key': idempotencyKey,
                    },
                    body: JSON.stringify({
                        amount,
                        ...(note ? { note } : {}),
                        idempotency_key: idempotencyKey,
                    }),
                },
            );

            setResellers((prev) =>
                prev.map((item) =>
                    item.telegram_id === telegramIdNum
                        ? {
                            ...item,
                            balance_owed: result.balance_owed,
                            remaining_credit: result.remaining_credit,
                        }
                        : item,
                ),
            );
            setSettlementAmounts((prev) => ({ ...prev, [telegramIdNum]: '' }));
            setSettlementNotes((prev) => ({ ...prev, [telegramIdNum]: '' }));
            setActionSuccess(t('admin_resellers_settlement_success'));
            await refreshListAndMaybeLedger(telegramIdNum);
        } catch (err) {
            if (isAPIStatus(err, 401)) {
                clearTelegramSession();
                setAuthExpired(true);
                return;
            }
            if (isAPIStatus(err, 403)) {
                setAccessDenied(true);
                return;
            }
            if (err instanceof APIError && err.body) {
                setActionError(err.body);
                return;
            }
            setActionError(t('admin_resellers_settlement_error'));
        } finally {
            setRowBusyId(null);
        }
    };

    const loadLedger = async (telegramIdNum: number) => {
        if (!initData) return;

        if (ledgerOpenId === telegramIdNum) {
            setLedgerOpenId(null);
            return;
        }

        setLedgerOpenId(telegramIdNum);
        setLedgerLoadingId(telegramIdNum);
        setActionError(null);

        try {
            const ledger = await fetchJSONWithTelegramAuth<ResellerLedgerEntry[]>(
                `/api/admin/customers/${encodeURIComponent(String(telegramIdNum))}/ledger`,
                initData,
            );
            setLedgerById((prev) => ({
                ...prev,
                [telegramIdNum]: Array.isArray(ledger) ? ledger : [],
            }));
        } catch (err) {
            if (isAPIStatus(err, 401)) {
                clearTelegramSession();
                setAuthExpired(true);
                return;
            }
            if (isAPIStatus(err, 403)) {
                setAccessDenied(true);
                return;
            }
            if (err instanceof APIError && err.body) {
                setActionError(err.body);
                return;
            }
            setActionError(t('admin_resellers_ledger_error'));
            setLedgerOpenId(null);
        } finally {
            setLedgerLoadingId(null);
        }
    };

    if (loading) return <LoadingScreen message={t('admin_resellers_loading')} />;
    if (authExpired) {
        return (
            <SessionExpiredScreen
                title={t('session_expired_title')}
                message={t('session_expired_desc')}
                reloadLabel={t('session_expired_reload')}
                closeLabel={t('session_expired_close')}
                onReload={() => { window.location.reload(); }}
                onClose={() => { close(); }}
            />
        );
    }
    if (!initData) {
        return (
            <div className="screen-center">
                <div style={{ fontSize: 48 }}>📱</div>
                <h1 style={{ fontSize: 20, fontWeight: 700, margin: 0 }}>Wavy Private Server Shop</h1>
                <p className="text-hint" style={{ margin: 0 }}>{t('open_in_tg')}</p>
            </div>
        );
    }
    if (accessDenied) {
        return <ErrorScreen message={t('admin_resellers_forbidden')} />;
    }
    if (error) {
        return (
            <ErrorScreen
                message={error}
                onRetry={() => { void loadResellers(); }}
                retryLabel={t('retry')}
            />
        );
    }

    return (
        <div className="animate-fade-in admin-promo-shell">
            <section className="digital-card admin-promo-hero">
                <div style={{ display: 'grid', gap: 14 }}>
                    <div>
                        <h1 style={{ fontSize: 22, fontWeight: 800, margin: 0, color: 'var(--digital-card-text)' }}>
                            {t('admin_resellers_title')}
                        </h1>
                        <p style={{ fontSize: 13, lineHeight: 1.5, margin: '8px 0 0', color: 'var(--digital-card-hint)' }}>
                            {t('admin_resellers_subtitle')}
                        </p>
                    </div>
                    <div className="admin-promo-stat-grid">
                        <div className="admin-promo-stat-card">
                            <span className="admin-promo-stat-label">{t('admin_resellers_total_label')}</span>
                            <strong className="admin-promo-stat-value">{resellers.length}</strong>
                        </div>
                    </div>
                </div>
            </section>

            <section className="glass-card admin-promo-panel">
                <div style={{ display: 'grid', gap: 6, marginBottom: 18 }}>
                    <h2 style={{ fontSize: 16, margin: 0 }}>{t('admin_resellers_toggle_title')}</h2>
                    <p className="text-hint" style={{ fontSize: 12, margin: 0 }}>{t('admin_resellers_toggle_caption')}</p>
                </div>

                <form className="admin-promo-form" onSubmit={handleEnable}>
                    <label className="admin-promo-field admin-promo-field-full">
                        <span className="admin-promo-label">{t('admin_resellers_telegram_id_label')}</span>
                        <input
                            className="admin-promo-input"
                            type="text"
                            inputMode="numeric"
                            value={telegramId}
                            onChange={(event) => setTelegramId(event.target.value)}
                            aria-label={t('admin_resellers_telegram_id_label')}
                            placeholder="123456789"
                            required
                        />
                    </label>
                    <div style={{ display: 'flex', gap: 10, gridColumn: '1 / -1', flexWrap: 'wrap' }}>
                        <button
                            className="btn-primary"
                            type="submit"
                            disabled={submitting}
                            style={{ flex: 1, minWidth: 140 }}
                        >
                            {submitting ? t('admin_resellers_saving') : t('admin_resellers_enable')}
                        </button>
                        <button
                            className="btn-secondary"
                            type="button"
                            disabled={submitting}
                            style={{ flex: 1, minWidth: 140 }}
                            onClick={() => { playClick(); void toggleReseller(false); }}
                        >
                            {submitting ? t('admin_resellers_saving') : t('admin_resellers_disable')}
                        </button>
                    </div>
                </form>
            </section>

            {actionSuccess && (
                <div role="status" className="glass-card" style={{ padding: 12, color: 'var(--color-success)' }}>
                    {actionSuccess}
                </div>
            )}
            {actionError && (
                <div role="alert" className="glass-card" style={{ padding: 12, color: 'var(--color-danger)' }}>
                    {actionError}
                </div>
            )}

            <section className="glass-card admin-promo-panel">
                <div style={{ display: 'grid', gap: 6, marginBottom: 18 }}>
                    <h2 style={{ fontSize: 16, margin: 0 }}>{t('admin_resellers_list_title')}</h2>
                    <p className="text-hint" style={{ fontSize: 12, margin: 0 }}>{t('admin_resellers_list_subtitle')}</p>
                </div>

                {resellers.length === 0 ? (
                    <div className="glass-card" style={{ padding: 18, display: 'grid', gap: 6 }}>
                        <strong>{t('admin_resellers_empty')}</strong>
                        <span className="text-hint" style={{ fontSize: 12 }}>{t('admin_resellers_empty_detail')}</span>
                    </div>
                ) : (
                    <div style={{ display: 'grid', gap: 12 }}>
                        {resellers.map((reseller) => {
                            const isExpanded = expandedId === reseller.telegram_id;
                            const isBusy = rowBusyId === reseller.telegram_id;
                            const ledgerOpen = ledgerOpenId === reseller.telegram_id;
                            const ledgerEntries = ledgerById[reseller.telegram_id] ?? [];

                            return (
                                <div key={reseller.telegram_id} className="glass-card admin-promo-list-card">
                                    <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, alignItems: 'flex-start' }}>
                                        <div style={{ minWidth: 0, flex: 1 }}>
                                            <div style={{ fontSize: 16, fontWeight: 700 }}>{reseller.telegram_id}</div>
                                            <div className="text-hint" style={{ fontSize: 12, marginTop: 6, display: 'grid', gap: 2 }}>
                                                <span>
                                                    {t('reseller_credit_limit')}: {formatMoney(reseller.credit_limit ?? 0)}
                                                </span>
                                                <span style={{ color: (reseller.balance_owed ?? 0) > 0 ? '#ff3b30' : undefined }}>
                                                    {t('reseller_balance_owed')}: {formatMoney(reseller.balance_owed ?? 0)}
                                                </span>
                                                <span style={{ color: 'var(--color-success)' }}>
                                                    {t('reseller_remaining_credit')}: {formatMoney(reseller.remaining_credit ?? 0)}
                                                </span>
                                            </div>
                                        </div>
                                        <span
                                            style={{
                                                padding: '6px 10px',
                                                borderRadius: 999,
                                                border: '1px solid rgba(52, 199, 89, 0.22)',
                                                background: 'rgba(52, 199, 89, 0.12)',
                                                color: 'var(--color-success)',
                                                fontSize: 11,
                                                fontWeight: 700,
                                                flexShrink: 0,
                                            }}
                                        >
                                            {t('admin_resellers_status_active')}
                                        </span>
                                    </div>

                                    <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', marginTop: 12 }}>
                                        <button
                                            type="button"
                                            className="btn-secondary"
                                            style={{ flex: 1, minWidth: 120, fontSize: 13, padding: '8px 12px' }}
                                            onClick={() => {
                                                playClick();
                                                setExpandedId(isExpanded ? null : reseller.telegram_id);
                                            }}
                                        >
                                            {isExpanded
                                                ? t('admin_resellers_hide_actions')
                                                : t('admin_resellers_manage_credit')}
                                        </button>
                                        <button
                                            type="button"
                                            className="btn-secondary"
                                            style={{ flex: 1, minWidth: 120, fontSize: 13, padding: '8px 12px' }}
                                            onClick={() => {
                                                playClick();
                                                void loadLedger(reseller.telegram_id);
                                            }}
                                            disabled={ledgerLoadingId === reseller.telegram_id}
                                        >
                                            {ledgerLoadingId === reseller.telegram_id
                                                ? t('admin_resellers_ledger_loading')
                                                : ledgerOpen
                                                    ? t('admin_resellers_hide_ledger')
                                                    : t('admin_resellers_view_ledger')}
                                        </button>
                                    </div>

                                    {isExpanded && (
                                        <div style={{ display: 'grid', gap: 14, marginTop: 14 }}>
                                            <form
                                                className="admin-promo-form"
                                                onSubmit={(event) => {
                                                    event.preventDefault();
                                                    playClick();
                                                    void setCreditLimit(reseller.telegram_id);
                                                }}
                                            >
                                                <label className="admin-promo-field admin-promo-field-full">
                                                    <span className="admin-promo-label">{t('admin_resellers_credit_limit_label')}</span>
                                                    <input
                                                        className="admin-promo-input"
                                                        type="number"
                                                        inputMode="decimal"
                                                        min={0}
                                                        step="any"
                                                        value={creditLimitDrafts[reseller.telegram_id] ?? ''}
                                                        onChange={(event) => {
                                                            setCreditLimitDrafts((prev) => ({
                                                                ...prev,
                                                                [reseller.telegram_id]: event.target.value,
                                                            }));
                                                        }}
                                                        aria-label={t('admin_resellers_credit_limit_label')}
                                                        disabled={isBusy}
                                                    />
                                                </label>
                                                <button
                                                    className="btn-primary"
                                                    type="submit"
                                                    disabled={isBusy}
                                                    style={{ gridColumn: '1 / -1' }}
                                                >
                                                    {isBusy ? t('admin_resellers_saving') : t('admin_resellers_set_credit_limit')}
                                                </button>
                                            </form>

                                            <form
                                                className="admin-promo-form"
                                                onSubmit={(event) => {
                                                    event.preventDefault();
                                                    playClick();
                                                    void recordSettlement(reseller.telegram_id);
                                                }}
                                            >
                                                <div style={{ gridColumn: '1 / -1', display: 'grid', gap: 4, marginBottom: 4 }}>
                                                    <strong style={{ fontSize: 13 }}>{t('admin_resellers_settlement_title')}</strong>
                                                    <span className="text-hint" style={{ fontSize: 11 }}>
                                                        {t('admin_resellers_settlement_caption')}
                                                    </span>
                                                </div>
                                                <label className="admin-promo-field admin-promo-field-full">
                                                    <span className="admin-promo-label">{t('admin_resellers_settlement_amount_label')}</span>
                                                    <input
                                                        className="admin-promo-input"
                                                        type="number"
                                                        inputMode="decimal"
                                                        min={0}
                                                        step="any"
                                                        value={settlementAmounts[reseller.telegram_id] ?? ''}
                                                        onChange={(event) => {
                                                            setSettlementAmounts((prev) => ({
                                                                ...prev,
                                                                [reseller.telegram_id]: event.target.value,
                                                            }));
                                                        }}
                                                        aria-label={t('admin_resellers_settlement_amount_label')}
                                                        disabled={isBusy}
                                                    />
                                                </label>
                                                <label className="admin-promo-field admin-promo-field-full">
                                                    <span className="admin-promo-label">{t('admin_resellers_settlement_note_label')}</span>
                                                    <input
                                                        className="admin-promo-input"
                                                        type="text"
                                                        value={settlementNotes[reseller.telegram_id] ?? ''}
                                                        onChange={(event) => {
                                                            setSettlementNotes((prev) => ({
                                                                ...prev,
                                                                [reseller.telegram_id]: event.target.value,
                                                            }));
                                                        }}
                                                        aria-label={t('admin_resellers_settlement_note_label')}
                                                        placeholder={t('admin_resellers_settlement_note_placeholder')}
                                                        disabled={isBusy}
                                                    />
                                                </label>
                                                <button
                                                    className="btn-primary"
                                                    type="submit"
                                                    disabled={isBusy}
                                                    style={{ gridColumn: '1 / -1' }}
                                                >
                                                    {isBusy
                                                        ? t('admin_resellers_saving')
                                                        : t('admin_resellers_record_settlement')}
                                                </button>
                                            </form>
                                        </div>
                                    )}

                                    {ledgerOpen && (
                                        <div style={{ marginTop: 14, display: 'grid', gap: 8 }}>
                                            <strong style={{ fontSize: 13 }}>{t('reseller_ledger_title')}</strong>
                                            {ledgerLoadingId === reseller.telegram_id ? (
                                                <span className="text-hint" style={{ fontSize: 12 }}>
                                                    {t('admin_resellers_ledger_loading')}
                                                </span>
                                            ) : ledgerEntries.length === 0 ? (
                                                <span className="text-hint" style={{ fontSize: 12 }}>
                                                    {t('reseller_ledger_empty')}
                                                </span>
                                            ) : (
                                                ledgerEntries.map((entry) => (
                                                    <div
                                                        key={entry.id}
                                                        className="glass-card"
                                                        style={{ padding: 12 }}
                                                    >
                                                        <div style={{
                                                            display: 'flex',
                                                            justifyContent: 'space-between',
                                                            gap: 12,
                                                            alignItems: 'flex-start',
                                                        }}>
                                                            <div style={{ flex: 1, minWidth: 0 }}>
                                                                <div style={{
                                                                    fontSize: 13,
                                                                    fontWeight: 600,
                                                                    textTransform: 'capitalize',
                                                                }}>
                                                                    {entry.entry_type.replace(/_/g, ' ')}
                                                                </div>
                                                                <div className="text-hint" style={{ fontSize: 11, marginTop: 2 }}>
                                                                    {new Date(entry.effective_at).toLocaleString()}
                                                                </div>
                                                                {entry.note && (
                                                                    <div className="text-hint" style={{
                                                                        fontSize: 12,
                                                                        marginTop: 4,
                                                                        lineHeight: 1.4,
                                                                    }}>
                                                                        {entry.note}
                                                                    </div>
                                                                )}
                                                            </div>
                                                            <div style={{
                                                                fontSize: 14,
                                                                fontWeight: 700,
                                                                color: ledgerAmountColor(entry),
                                                                flexShrink: 0,
                                                            }}>
                                                                {formatLedgerAmount(entry)}
                                                            </div>
                                                        </div>
                                                    </div>
                                                ))
                                            )}
                                        </div>
                                    )}
                                </div>
                            );
                        })}
                    </div>
                )}
            </section>
        </div>
    );
}
