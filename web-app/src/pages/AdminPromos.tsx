import { FormEvent, useCallback, useEffect, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { ErrorScreen } from '../components/ErrorScreen';
import { LoadingScreen } from '../components/LoadingScreen';
import { SessionExpiredScreen } from '../components/SessionExpiredScreen';
import { clearTelegramSession, fetchJSONWithTelegramAuth, fetchWithTelegramAuth } from '../lib/auth';
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
            const meData = await fetchJSONWithTelegramAuth<UserData>('/api/me', initData);
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

    const promoStatusLabel = (promo: AdminPromo) => {
        if (promo.status === 'exhausted') return t('admin_promos_status_exhausted');
        if (promo.status === 'expired') return t('admin_promos_status_expired');
        if (promo.status === 'active') return t('admin_promos_status_active');
        if (promo.used_count >= promo.max_uses) return t('admin_promos_status_exhausted');
        if (new Date(promo.valid_until).getTime() <= Date.now()) return t('admin_promos_status_expired');
        return t('admin_promos_status_active');
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

    return (
        <div className="animate-fade-in" style={{ padding: 16, display: 'flex', flexDirection: 'column', gap: 16, minHeight: '100vh' }}>
            <div style={{ textAlign: 'center', padding: '8px 0' }}>
                <h1 style={{ fontSize: 20, fontWeight: 700, margin: 0 }}>{t('admin_promos_title')}</h1>
                <p className="text-hint" style={{ fontSize: 12, margin: '6px 0 0' }}>{t('admin_promos_subtitle')}</p>
            </div>

            <form className="glass-card" style={{ padding: 16, display: 'grid', gap: 12 }} onSubmit={(event) => { playClick(); void handleCreatePromo(event); }}>
                <label style={{ display: 'grid', gap: 6 }}>
                    <span style={{ fontSize: 13, fontWeight: 600 }}>{t('admin_promos_code_label')}</span>
                    <input
                        type="text"
                        value={form.code}
                        onChange={(event) => setForm((prev) => ({ ...prev, code: event.target.value }))}
                        required
                        aria-label={t('admin_promos_code_label')}
                    />
                </label>
                <label style={{ display: 'grid', gap: 6 }}>
                    <span style={{ fontSize: 13, fontWeight: 600 }}>{t('admin_promos_discount_label')}</span>
                    <input
                        type="number"
                        min="1"
                        max="100"
                        value={form.discountPercent}
                        onChange={(event) => setForm((prev) => ({ ...prev, discountPercent: event.target.value }))}
                        required
                        aria-label={t('admin_promos_discount_label')}
                    />
                </label>
                <label style={{ display: 'grid', gap: 6 }}>
                    <span style={{ fontSize: 13, fontWeight: 600 }}>{t('admin_promos_days_label')}</span>
                    <input
                        type="number"
                        min="1"
                        value={form.validDays}
                        onChange={(event) => setForm((prev) => ({ ...prev, validDays: event.target.value }))}
                        required
                        aria-label={t('admin_promos_days_label')}
                    />
                </label>
                <label style={{ display: 'grid', gap: 6 }}>
                    <span style={{ fontSize: 13, fontWeight: 600 }}>{t('admin_promos_uses_label')}</span>
                    <input
                        type="number"
                        min="1"
                        value={form.maxUses}
                        onChange={(event) => setForm((prev) => ({ ...prev, maxUses: event.target.value }))}
                        required
                        aria-label={t('admin_promos_uses_label')}
                    />
                </label>
                <button className="btn-primary" type="submit" disabled={submitting}>
                    {submitting ? t('admin_promos_creating') : t('admin_promos_create')}
                </button>
            </form>

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

            <div style={{ display: 'grid', gap: 12 }}>
                {promos.length === 0 && (
                    <div className="glass-card" style={{ padding: 16 }}>
                        {t('admin_promos_empty')}
                    </div>
                )}

                {promos.map((promo) => (
                    <div key={promo.code} className="glass-card" style={{ padding: 16, display: 'grid', gap: 10 }}>
                        <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, alignItems: 'center' }}>
                            <div>
                                <div style={{ fontSize: 16, fontWeight: 700 }}>{promo.code}</div>
                                <div className="text-hint" style={{ fontSize: 12 }}>
                                    {promo.discount_percent}% off · {promoStatusLabel(promo)}
                                </div>
                            </div>
                            <button
                                className="btn-secondary"
                                type="button"
                                onClick={() => { playClick(); void handleDeletePromo(promo.code); }}
                                disabled={deletingCode === promo.code}
                                aria-label={`${t('admin_promos_delete')} ${promo.code}`}
                            >
                                {deletingCode === promo.code ? t('admin_promos_deleting') : t('admin_promos_delete')}
                            </button>
                        </div>
                        <div className="text-hint" style={{ fontSize: 12 }}>
                            {promo.used_count}/{promo.max_uses} uses · {new Date(promo.valid_until).toLocaleDateString(language === 'en' ? 'en-US' : 'my-MM', {
                                year: 'numeric',
                                month: 'short',
                                day: 'numeric',
                            })}
                        </div>
                    </div>
                ))}
            </div>
        </div>
    );
}
