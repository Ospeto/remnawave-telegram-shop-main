import { FormEvent, useCallback, useEffect, useState } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { ErrorScreen } from '../components/ErrorScreen';
import { LoadingScreen } from '../components/LoadingScreen';
import { SessionExpiredScreen } from '../components/SessionExpiredScreen';
import {
    clearTelegramSession,
    fetchJSONWithTelegramAuth,
    fetchUserScopedJSONWithTelegramAuth,
} from '../lib/auth';
import { APIError, isAPIStatus } from '../lib/http';
import { useLanguage } from '../lib/LanguageContext';
import {
    ResellerAccount as ResellerAccountData,
    ResellerLedgerEntry,
    ResellerSettlementResponse,
    UserData,
} from '../lib/types';
import { createIdempotencyKey, useTelegram } from '../lib/twa';

function formatMoney(value: number): string {
    return (value || 0).toLocaleString();
}

// direction increase → +amount (owed up / red)
// direction decrease → -amount (owed down / green)
// Prefer direction; entry_type is secondary label only
function resolveLedgerSignedAmount(entry: ResellerLedgerEntry): number {
    const dir = entry.direction.toLowerCase();
    if (dir === 'decrease') return -Math.abs(entry.amount);
    if (dir === 'increase') return Math.abs(entry.amount);
    // fallback: settlement decreases, sale increases
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

export function ResellerAccount() {
    const { tg, initData, close } = useTelegram();
    const { t } = useLanguage();
    const navigate = useNavigate();

    const [account, setAccount] = useState<ResellerAccountData | null>(null);
    const [ledger, setLedger] = useState<ResellerLedgerEntry[]>([]);
    const [loading, setLoading] = useState(true);
    const [loadError, setLoadError] = useState<string | null>(null);
    const [authExpired, setAuthExpired] = useState(false);
    const [accessDenied, setAccessDenied] = useState(false);

    const [payAmount, setPayAmount] = useState('');
    const [paying, setPaying] = useState(false);
    const [payError, setPayError] = useState<string | null>(null);
    const [paySuccess, setPaySuccess] = useState<string | null>(null);

    const handleBack = useCallback(() => navigate('/'), [navigate]);

    const loadAccountData = useCallback(async () => {
        if (!initData) {
            setLoading(false);
            return;
        }

        setLoading(true);
        setLoadError(null);
        setAuthExpired(false);
        setAccessDenied(false);

        try {
            const meData = await fetchUserScopedJSONWithTelegramAuth<UserData>(
                '/api/me',
                initData,
                tg?.initDataUnsafe?.user?.id,
            );

            if (!meData.is_reseller) {
                setAccessDenied(true);
                return;
            }

            const [accountData, ledgerData] = await Promise.all([
                fetchJSONWithTelegramAuth<ResellerAccountData>('/api/reseller/account', initData),
                fetchJSONWithTelegramAuth<ResellerLedgerEntry[]>('/api/reseller/ledger?limit=50&offset=0', initData),
            ]);

            setAccount(accountData);
            setLedger(Array.isArray(ledgerData) ? ledgerData : []);
            setPayAmount(
                accountData.balance_owed > 0 ? String(accountData.balance_owed) : '',
            );
        } catch (err) {
            console.warn('Reseller account load error:', err);
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
                setLoadError(err.body);
                return;
            }
            setLoadError(t('reseller_account_error'));
        } finally {
            setLoading(false);
        }
    }, [initData, t, tg]);

    useEffect(() => {
        if (!tg) return;
        tg.BackButton.show();
        tg.BackButton.onClick(handleBack);
        return () => tg.BackButton.offClick(handleBack);
    }, [tg, handleBack]);

    useEffect(() => {
        void loadAccountData();
    }, [loadAccountData]);

    const parsedAmount = Number(payAmount);
    const canPay =
        !!account &&
        account.balance_owed > 0 &&
        Number.isFinite(parsedAmount) &&
        parsedAmount > 0 &&
        parsedAmount <= account.balance_owed &&
        !paying;

    const handlePay = async (event: FormEvent) => {
        event.preventDefault();
        if (!initData || !canPay) return;

        setPaying(true);
        setPayError(null);
        setPaySuccess(null);

        const idempotencyKey = createIdempotencyKey();

        try {
            const result = await fetchJSONWithTelegramAuth<ResellerSettlementResponse>(
                '/api/reseller/settlements',
                initData,
                {
                    method: 'POST',
                    headers: {
                        'Content-Type': 'application/json',
                        'Idempotency-Key': idempotencyKey,
                    },
                    body: JSON.stringify({
                        amount: parsedAmount,
                        payment_method: 'wallet',
                        idempotency_key: idempotencyKey,
                    }),
                },
            );

            setAccount((prev) =>
                prev
                    ? {
                        ...prev,
                        balance_owed: result.balance_owed,
                        remaining_credit: result.remaining_credit,
                    }
                    : prev,
            );
            setPaySuccess(t('reseller_pay_success'));
            setPayAmount(result.balance_owed > 0 ? String(result.balance_owed) : '');

            try {
                const ledgerData = await fetchJSONWithTelegramAuth<ResellerLedgerEntry[]>(
                    '/api/reseller/ledger?limit=50&offset=0',
                    initData,
                );
                setLedger(Array.isArray(ledgerData) ? ledgerData : []);
            } catch {
                // Keep updated balances even if ledger refresh fails.
            }
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
                setPayError(err.body);
                return;
            }
            setPayError(t('reseller_pay_error'));
        } finally {
            setPaying(false);
        }
    };

    if (!initData) {
        return (
            <div className="screen-center">
                <div style={{ fontSize: 48 }}>📱</div>
                <h1 style={{ fontSize: 20, fontWeight: 700, margin: 0 }}>Wavy Private Server Shop</h1>
                <p className="text-hint" style={{ margin: 0 }}>{t('open_in_tg')}</p>
            </div>
        );
    }

    if (loading) return <LoadingScreen message={t('reseller_account_loading')} />;

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

    if (accessDenied) {
        return (
            <div className="page-wrapper animate-fade-in" style={{ textAlign: 'center', paddingTop: 48 }}>
                <div style={{ fontSize: 40, marginBottom: 12 }} aria-hidden="true">🔒</div>
                <h1 style={{ fontSize: 18, fontWeight: 700, margin: '0 0 8px' }}>
                    {t('reseller_access_required')}
                </h1>
                <Link to="/" className="btn-primary" style={{ display: 'inline-block', marginTop: 16, textDecoration: 'none', padding: '12px 20px' }}>
                    {t('go_home')}
                </Link>
            </div>
        );
    }

    if (loadError || !account) {
        return (
            <ErrorScreen
                message={loadError || t('reseller_account_error')}
                onRetry={() => { void loadAccountData(); }}
                retryLabel={t('retry')}
            />
        );
    }

    return (
        <div className="page-wrapper animate-fade-in">
            <div style={{ textAlign: 'center', padding: '8px 0' }}>
                <h1 style={{ fontSize: 'var(--font-h1)', fontWeight: 'var(--weight-bold)', margin: 0 }}>
                    {t('reseller_account_title')}
                </h1>
                <p className="text-hint" style={{ fontSize: 'var(--font-caption)', margin: '6px 0 0' }}>
                    {t('reseller_account_subtitle')}
                </p>
            </div>

            <div
                className="animate-slide-up"
                style={{
                    display: 'grid',
                    gridTemplateColumns: '1fr',
                    gap: 10,
                    marginBottom: 8,
                }}
            >
                <div className="glass-card" style={{ padding: 16 }}>
                    <div className="text-hint" style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', marginBottom: 4 }}>
                        {t('reseller_credit_limit')}
                    </div>
                    <div style={{ fontSize: 22, fontWeight: 800 }}>{formatMoney(account.credit_limit)}</div>
                </div>
                <div className="glass-card" style={{ padding: 16 }}>
                    <div className="text-hint" style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', marginBottom: 4 }}>
                        {t('reseller_balance_owed')}
                    </div>
                    <div style={{ fontSize: 22, fontWeight: 800, color: account.balance_owed > 0 ? '#ff3b30' : 'var(--text-color)' }}>
                        {formatMoney(account.balance_owed)}
                    </div>
                </div>
                <div className="glass-card" style={{ padding: 16 }}>
                    <div className="text-hint" style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', marginBottom: 4 }}>
                        {t('reseller_remaining_credit')}
                    </div>
                    <div style={{ fontSize: 22, fontWeight: 800, color: 'var(--color-success)' }}>
                        {formatMoney(account.remaining_credit)}
                    </div>
                </div>
            </div>

            <div
                className="glass-card animate-slide-up"
                style={{ padding: 16, marginBottom: 8 }}
            >
                <h2 style={{ fontSize: 16, fontWeight: 700, margin: '0 0 12px' }}>
                    {t('reseller_pay_balance')}
                </h2>

                {paySuccess && (
                    <div
                        role="status"
                        style={{
                            marginBottom: 12,
                            padding: '10px 12px',
                            borderRadius: 10,
                            background: 'rgba(52, 199, 89, 0.12)',
                            color: 'var(--text-color)',
                            fontSize: 13,
                            lineHeight: 1.5,
                        }}
                    >
                        {paySuccess}
                    </div>
                )}

                {payError && (
                    <div
                        role="alert"
                        style={{
                            marginBottom: 12,
                            padding: '10px 12px',
                            borderRadius: 10,
                            background: 'rgba(255, 59, 48, 0.12)',
                            color: 'var(--text-color)',
                            fontSize: 13,
                            lineHeight: 1.5,
                        }}
                    >
                        {payError}
                    </div>
                )}

                <form onSubmit={(e) => { void handlePay(e); }}>
                    <label style={{ display: 'block', marginBottom: 12 }}>
                        <span className="text-hint" style={{ fontSize: 12, fontWeight: 600, display: 'block', marginBottom: 6 }}>
                            {t('reseller_pay_amount')}
                        </span>
                        <input
                            type="number"
                            inputMode="decimal"
                            min={0}
                            step="any"
                            value={payAmount}
                            onChange={(e) => {
                                setPayAmount(e.target.value);
                                setPayError(null);
                                setPaySuccess(null);
                            }}
                            disabled={account.balance_owed <= 0 || paying}
                            aria-label={t('reseller_pay_amount')}
                            style={{
                                width: '100%',
                                padding: '12px 14px',
                                borderRadius: 10,
                                border: '1px solid var(--border-color)',
                                background: 'var(--main-bg)',
                                color: 'var(--text-color)',
                                fontSize: 16,
                                fontWeight: 600,
                                boxSizing: 'border-box',
                            }}
                        />
                    </label>
                    <button
                        type="submit"
                        className="btn-primary"
                        disabled={!canPay}
                        style={{
                            width: '100%',
                            padding: '12px',
                            opacity: canPay ? 1 : 0.55,
                            cursor: canPay ? 'pointer' : 'not-allowed',
                        }}
                    >
                        {paying ? t('reseller_pay_processing') : t('reseller_pay_submit')}
                    </button>
                </form>
            </div>

            <div>
                <h2 style={{ fontSize: 'var(--font-h2)', fontWeight: 'var(--weight-bold)', margin: '16px 0 12px' }}>
                    {t('reseller_ledger_title')}
                </h2>

                {ledger.length === 0 ? (
                    <div className="glass-card" style={{ padding: '24px', textAlign: 'center' }}>
                        <div className="text-hint" style={{ fontSize: 13 }}>{t('reseller_ledger_empty')}</div>
                    </div>
                ) : (
                    <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                        {ledger.map((entry) => (
                            <div key={entry.id} className="glass-card" style={{ padding: 14 }}>
                                <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, alignItems: 'flex-start' }}>
                                    <div style={{ flex: 1, minWidth: 0 }}>
                                        <div style={{ fontSize: 'var(--font-body)', fontWeight: 600, textTransform: 'capitalize' }}>
                                            {entry.entry_type.replace(/_/g, ' ')}
                                        </div>
                                        <div className="text-hint" style={{ fontSize: 11, marginTop: 2 }}>
                                            {new Date(entry.effective_at).toLocaleString()}
                                        </div>
                                        {entry.note && (
                                            <div className="text-hint" style={{ fontSize: 12, marginTop: 6, lineHeight: 1.4 }}>
                                                {entry.note}
                                            </div>
                                        )}
                                    </div>
                                    <div style={{
                                        fontSize: 15,
                                        fontWeight: 700,
                                        color: ledgerAmountColor(entry),
                                        flexShrink: 0,
                                    }}>
                                        {formatLedgerAmount(entry)}
                                    </div>
                                </div>
                            </div>
                        ))}
                    </div>
                )}
            </div>

            <div style={{ height: 32 }} />
        </div>
    );
}
