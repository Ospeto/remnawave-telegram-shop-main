import { FormEvent, useCallback, useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { ErrorScreen } from '../components/ErrorScreen';
import { LoadingScreen } from '../components/LoadingScreen';
import { SessionExpiredScreen } from '../components/SessionExpiredScreen';
import { clearTelegramSession, fetchJSONWithTelegramAuth, fetchUserScopedJSONWithTelegramAuth, fetchWithTelegramAuth } from '../lib/auth';
import { APIError, isAPIStatus } from '../lib/http';
import { useLanguage } from '../lib/LanguageContext';
import { AdminPromo, UserData } from '../lib/types';
import { useMXBrownSound } from '../lib/useMXBrownSound';
import { useTelegram } from '../lib/twa';

interface PromoFormState {
    code: string;
    discountPercent: string;
    validDays: string;
    maxUses: string;
}

const INITIAL_FORM: PromoFormState = {
    code: '',
    discountPercent: '',
    validDays: '',
    maxUses: '',
};

export function AdminPromos() {
    const { tg, initData, close } = useTelegram();
    const { t, language } = useLanguage();
    const navigate = useNavigate();
    const { playClick } = useMXBrownSound();
    const [promos, setPromos] = useState<AdminPromo[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [authExpired, setAuthExpired] = useState(false);
    const [accessDenied, setAccessDenied] = useState(false);
    const [form, setForm] = useState<PromoFormState>(INITIAL_FORM);
    const [submitting, setSubmitting] = useState(false);
    const [deletingCode, setDeletingCode] = useState<string | null>(null);
    const [actionError, setActionError] = useState<string | null>(null);
    const [actionSuccess, setActionSuccess] = useState<string | null>(null);

    const handleBack = useCallback(() => {
        navigate('/');
    }, [navigate]);

    const loadPromos = useCallback(async () => {
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

            const promoData = await fetchJSONWithTelegramAuth<AdminPromo[]>('/api/admin/promos', initData);
            setPromos(Array.isArray(promoData) ? promoData : []);
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
            setError(err instanceof Error ? err.message : t('admin_promos_load_error'));
        } finally {
            setLoading(false);
        }
    }, [initData, t]);

    useEffect(() => {
        if (!tg) return;
        tg.BackButton.show();
        tg.BackButton.onClick(handleBack);
        return () => tg.BackButton.offClick(handleBack);
    }, [handleBack, tg]);

    useEffect(() => {
        void loadPromos();
    }, [loadPromos]);

    const handleCreatePromo = async (event: FormEvent<HTMLFormElement>) => {
        event.preventDefault();
        if (!initData || submitting) return;

        setSubmitting(true);
        setActionError(null);
        setActionSuccess(null);

        try {
            const response = await fetchWithTelegramAuth('/api/admin/promos', initData, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({
                    code: form.code.trim(),
                    discount_percent: Number(form.discountPercent),
                    duration_days: Number(form.validDays),
                    max_uses: Number(form.maxUses),
                }),
            });

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
                setActionError(message || t('admin_promos_create_error'));
                return;
            }

            const createdPromo = await response.json() as AdminPromo;
            setPromos((prev) => [createdPromo, ...prev.filter((promo) => promo.code !== createdPromo.code)]);
            setForm(INITIAL_FORM);
            setActionSuccess(t('admin_promos_save_success'));
        } catch {
            setActionError(t('admin_promos_create_error'));
        } finally {
            setSubmitting(false);
        }
    };

    const handleDeletePromo = async (code: string) => {
        if (!initData || deletingCode === code) return;
        const confirmMessage = t('admin_promos_delete_confirm').replace('{{code}}', code);
        if (!window.confirm(confirmMessage)) return;

        setDeletingCode(code);
        setActionError(null);
        setActionSuccess(null);

        try {
            const response = await fetchWithTelegramAuth(`/api/admin/promos/${encodeURIComponent(code)}`, initData, {
                method: 'DELETE',
            });

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
                setActionError(message || t('admin_promos_delete_error'));
                return;
            }

            setPromos((prev) => prev.filter((promo) => promo.code !== code));
            setActionSuccess(t('admin_promos_delete_success'));
        } catch {
            setActionError(t('admin_promos_delete_error'));
        } finally {
            setDeletingCode(null);
        }
    };

    const promoStatusKey = (promo: AdminPromo) => {
        if (promo.status === 'exhausted' || promo.status === 'expired' || promo.status === 'active') {
            return promo.status;
        }
        if (promo.used_count >= promo.max_uses) return 'exhausted';
        if (new Date(promo.valid_until).getTime() <= Date.now()) return 'expired';
        return 'active';
    };

    const promoStatusLabel = (promo: AdminPromo) => {
        const status = promoStatusKey(promo);
        if (status === 'exhausted') return t('admin_promos_status_exhausted');
        if (status === 'expired') return t('admin_promos_status_expired');
        return t('admin_promos_status_active');
    };

    const promoTone = (promo: AdminPromo) => {
        const status = promoStatusKey(promo);
        if (status === 'active') {
            return {
                color: 'var(--color-success)',
                background: 'rgba(52, 199, 89, 0.12)',
                border: 'rgba(52, 199, 89, 0.22)',
            };
        }
        if (status === 'expired') {
            return {
                color: 'var(--tg-hint)',
                background: 'rgba(255, 255, 255, 0.06)',
                border: 'var(--card-border)',
            };
        }
        return {
            color: '#ff9f0a',
            background: 'rgba(255, 159, 10, 0.12)',
            border: 'rgba(255, 159, 10, 0.24)',
        };
    };

    if (loading) return <LoadingScreen message={t('admin_promos_loading')} />;
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
        return <ErrorScreen message={t('admin_promos_forbidden')} />;
    }
    if (error) {
        return (
            <ErrorScreen
                message={error}
                onRetry={() => { void loadPromos(); }}
                retryLabel={t('retry')}
            />
        );
    }

    const activeCount = promos.filter((promo) => promoStatusKey(promo) === 'active').length;
    const inactiveCount = promos.length - activeCount;

    return (
        <div className="animate-fade-in admin-promo-shell">
            <section className="digital-card admin-promo-hero">
                <div style={{ position: 'relative', zIndex: 1, display: 'grid', gap: 18 }}>
                    <div style={{ display: 'grid', gap: 6 }}>
                        <h1 style={{ fontSize: 22, fontWeight: 800, margin: 0, color: 'var(--digital-card-text)' }}>{t('admin_promos_title')}</h1>
                        <p style={{ fontSize: 13, lineHeight: 1.5, margin: 0, color: 'var(--digital-card-hint)' }}>
                            {t('admin_promos_subtitle')}
                        </p>
                    </div>

                    <div className="admin-promo-stat-grid">
                        <div className="admin-promo-stat-card">
                            <span className="admin-promo-stat-label">{t('admin_promos_total_label')}</span>
                            <strong className="admin-promo-stat-value">{promos.length}</strong>
                        </div>
                        <div className="admin-promo-stat-card">
                            <span className="admin-promo-stat-label">{t('admin_promos_active_label')}</span>
                            <strong className="admin-promo-stat-value">{activeCount}</strong>
                        </div>
                        <div className="admin-promo-stat-card">
                            <span className="admin-promo-stat-label">{t('admin_promos_inactive_label')}</span>
                            <strong className="admin-promo-stat-value">{inactiveCount}</strong>
                        </div>
                    </div>
                </div>
            </section>

            <section className="glass-card admin-promo-panel">
                <div style={{ display: 'grid', gap: 4 }}>
                    <h2 style={{ fontSize: 17, fontWeight: 700, margin: 0 }}>{t('admin_promos_create')}</h2>
                    <p className="text-hint" style={{ fontSize: 13, lineHeight: 1.5, margin: 0 }}>
                        {t('admin_promos_form_caption')}
                    </p>
                </div>

                <form className="admin-promo-form" onSubmit={(event) => { playClick(); void handleCreatePromo(event); }}>
                    <label className="admin-promo-field admin-promo-field-full">
                        <span className="admin-promo-label">{t('admin_promos_code_label')}</span>
                        <input
                            className="admin-promo-input"
                            type="text"
                            value={form.code}
                            onChange={(event) => setForm((prev) => ({ ...prev, code: event.target.value }))}
                            required
                            aria-label={t('admin_promos_code_label')}
                            placeholder="NEWYEAR30"
                        />
                    </label>
                    <label className="admin-promo-field">
                        <span className="admin-promo-label">{t('admin_promos_discount_label')}</span>
                        <input
                            className="admin-promo-input"
                            type="number"
                            min="1"
                            max="100"
                            value={form.discountPercent}
                            onChange={(event) => setForm((prev) => ({ ...prev, discountPercent: event.target.value }))}
                            required
                            aria-label={t('admin_promos_discount_label')}
                            placeholder="30"
                        />
                    </label>
                    <label className="admin-promo-field">
                        <span className="admin-promo-label">{t('admin_promos_days_label')}</span>
                        <input
                            className="admin-promo-input"
                            type="number"
                            min="1"
                            value={form.validDays}
                            onChange={(event) => setForm((prev) => ({ ...prev, validDays: event.target.value }))}
                            required
                            aria-label={t('admin_promos_days_label')}
                            placeholder="7"
                        />
                    </label>
                    <label className="admin-promo-field">
                        <span className="admin-promo-label">{t('admin_promos_uses_label')}</span>
                        <input
                            className="admin-promo-input"
                            type="number"
                            min="1"
                            value={form.maxUses}
                            onChange={(event) => setForm((prev) => ({ ...prev, maxUses: event.target.value }))}
                            required
                            aria-label={t('admin_promos_uses_label')}
                            placeholder="100"
                        />
                    </label>
                    <button className="btn-primary admin-promo-submit" type="submit" disabled={submitting}>
                        {submitting ? t('admin_promos_creating') : t('admin_promos_create')}
                    </button>
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

            <section style={{ display: 'grid', gap: 12 }}>
                <div style={{ display: 'grid', gap: 4 }}>
                    <h2 style={{ fontSize: 17, fontWeight: 700, margin: 0 }}>{t('admin_promos_list_title')}</h2>
                    <p className="text-hint" style={{ fontSize: 13, lineHeight: 1.5, margin: 0 }}>
                        {t('admin_promos_list_subtitle')}
                    </p>
                </div>

                {promos.length === 0 && (
                    <div className="glass-card" style={{ padding: 18, display: 'grid', gap: 6 }}>
                        <div style={{ fontSize: 15, fontWeight: 700 }}>{t('admin_promos_empty')}</div>
                        <div className="text-hint" style={{ fontSize: 13, lineHeight: 1.5 }}>{t('admin_promos_empty_detail')}</div>
                    </div>
                )}

                {promos.map((promo) => (
                    <div key={promo.code} className="glass-card admin-promo-list-card">
                        <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, alignItems: 'flex-start' }}>
                            <div style={{ display: 'grid', gap: 8 }}>
                                <div style={{ display: 'flex', flexWrap: 'wrap', alignItems: 'center', gap: 8 }}>
                                    <div style={{ fontSize: 17, fontWeight: 800, letterSpacing: '0.2px' }}>{promo.code}</div>
                                    <span
                                        style={{
                                            display: 'inline-flex',
                                            alignItems: 'center',
                                            borderRadius: 999,
                                            padding: '6px 10px',
                                            fontSize: 11,
                                            fontWeight: 700,
                                            border: `1px solid ${promoTone(promo).border}`,
                                            background: promoTone(promo).background,
                                            color: promoTone(promo).color,
                                            textTransform: 'uppercase',
                                            letterSpacing: '0.35px',
                                        }}
                                    >
                                        {promoStatusLabel(promo)}
                                    </span>
                                </div>
                                <div className="text-hint" style={{ fontSize: 13, lineHeight: 1.5 }}>
                                    {promo.discount_percent}% • {promo.used_count}/{promo.max_uses}
                                </div>
                            </div>
                            <button
                                className="btn-danger"
                                type="button"
                                onClick={() => { playClick(); void handleDeletePromo(promo.code); }}
                                disabled={deletingCode === promo.code}
                                aria-label={`${t('admin_promos_delete')} ${promo.code}`}
                            >
                                {deletingCode === promo.code ? t('admin_promos_deleting') : t('admin_promos_delete')}
                            </button>
                        </div>

                        <div className="admin-promo-metrics">
                            <div className="admin-promo-metric-card">
                                <span className="admin-promo-metric-label">{t('admin_promos_discount_metric')}</span>
                                <strong className="admin-promo-metric-value">{promo.discount_percent}%</strong>
                            </div>
                            <div className="admin-promo-metric-card">
                                <span className="admin-promo-metric-label">{t('admin_promos_usage_metric')}</span>
                                <strong className="admin-promo-metric-value">{promo.used_count}/{promo.max_uses}</strong>
                            </div>
                            <div className="admin-promo-metric-card">
                                <span className="admin-promo-metric-label">{t('admin_promos_expiry_metric')}</span>
                                <strong className="admin-promo-metric-value">
                                    {new Date(promo.valid_until).toLocaleDateString(language === 'en' ? 'en-US' : 'my-MM', {
                                        year: 'numeric',
                                        month: 'short',
                                        day: 'numeric',
                                    })}
                                </strong>
                            </div>
                        </div>
                    </div>
                ))}
            </section>
        </div>
    );
}
