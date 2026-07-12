import { FormEvent, useCallback, useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { ErrorScreen } from '../components/ErrorScreen';
import { LoadingScreen } from '../components/LoadingScreen';
import { SessionExpiredScreen } from '../components/SessionExpiredScreen';
import { clearTelegramSession, fetchJSONWithTelegramAuth, fetchUserScopedJSONWithTelegramAuth, fetchWithTelegramAuth } from '../lib/auth';
import { APIError, isAPIStatus } from '../lib/http';
import { useLanguage } from '../lib/LanguageContext';
import { UserData } from '../lib/types';
import { useMXBrownSound } from '../lib/useMXBrownSound';
import { useTelegram } from '../lib/twa';

interface AdminReseller {
    telegram_id: number;
    is_reseller: boolean;
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
            setResellers(Array.isArray(list) ? list : []);
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
                        {resellers.map((reseller) => (
                            <div key={reseller.telegram_id} className="glass-card admin-promo-list-card">
                                <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, alignItems: 'center' }}>
                                    <div>
                                        <div style={{ fontSize: 16, fontWeight: 700 }}>{reseller.telegram_id}</div>
                                        <div className="text-hint" style={{ fontSize: 12, marginTop: 4 }}>
                                            {t('admin_resellers_status_active')}
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
                                        }}
                                    >
                                        {t('admin_resellers_status_active')}
                                    </span>
                                </div>
                            </div>
                        ))}
                    </div>
                )}
            </section>
        </div>
    );
}
