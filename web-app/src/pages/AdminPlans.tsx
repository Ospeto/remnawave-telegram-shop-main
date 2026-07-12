import { FormEvent, useCallback, useEffect, useMemo, useState } from 'react';
import { useNavigate } from 'react-router-dom';
import { ErrorScreen } from '../components/ErrorScreen';
import { LoadingScreen } from '../components/LoadingScreen';
import { SessionExpiredScreen } from '../components/SessionExpiredScreen';
import { clearTelegramSession, fetchJSONWithTelegramAuth, fetchUserScopedJSONWithTelegramAuth, fetchWithTelegramAuth } from '../lib/auth';
import { APIError, isAPIStatus } from '../lib/http';
import { useLanguage } from '../lib/LanguageContext';
import { AdminPlan, UserData } from '../lib/types';
import { useMXBrownSound } from '../lib/useMXBrownSound';
import { useTelegram } from '../lib/twa';

interface PlanFormState {
    label: string;
    days: string;
    price: string;
    trafficLimitGB: string;
    wholesalePrice: string;
}

const INITIAL_FORM: PlanFormState = {
    label: '',
    days: '',
    price: '',
    trafficLimitGB: '0',
    wholesalePrice: '',
};

function formFromPlan(plan: AdminPlan): PlanFormState {
    return {
        label: plan.label,
        days: String(plan.days),
        price: String(plan.price),
        trafficLimitGB: String(plan.traffic_limit_gb),
        wholesalePrice: plan.wholesale_price != null ? String(plan.wholesale_price) : '',
    };
}

function parseWholesalePrice(value: string): number | null {
    const trimmed = value.trim();
    if (!trimmed) return null;
    return Number(trimmed);
}

function isWholeNumber(value: string): boolean {
    return /^\d+$/.test(value.trim());
}

export function AdminPlans() {
    const { tg, initData, close } = useTelegram();
    const { t } = useLanguage();
    const navigate = useNavigate();
    const { playClick } = useMXBrownSound();

    const [plans, setPlans] = useState<AdminPlan[]>([]);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [authExpired, setAuthExpired] = useState(false);
    const [accessDenied, setAccessDenied] = useState(false);
    const [form, setForm] = useState<PlanFormState>(INITIAL_FORM);
    const [editForms, setEditForms] = useState<Record<string, PlanFormState>>({});
    const [submitting, setSubmitting] = useState(false);
    const [savingId, setSavingId] = useState<string | null>(null);
    const [archivingId, setArchivingId] = useState<string | null>(null);
    const [actionError, setActionError] = useState<string | null>(null);
    const [actionSuccess, setActionSuccess] = useState<string | null>(null);

    const handleBack = useCallback(() => {
        navigate('/admin');
    }, [navigate]);

    const loadPlans = useCallback(async () => {
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

            const planData = await fetchJSONWithTelegramAuth<AdminPlan[]>('/api/admin/plans', initData);
            const normalizedPlans = Array.isArray(planData)
                ? [...planData].sort((a, b) => a.sort_order - b.sort_order)
                : [];
            setPlans(normalizedPlans);
            setEditForms(
                normalizedPlans.reduce<Record<string, PlanFormState>>((acc, plan) => {
                    acc[plan.id] = formFromPlan(plan);
                    return acc;
                }, {}),
            );
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
            setError(err instanceof Error ? err.message : t('admin_plans_load_error'));
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
        void loadPlans();
    }, [loadPlans]);

    const activePlans = useMemo(() => plans.filter((plan) => plan.active), [plans]);
    const archivedPlans = useMemo(() => plans.filter((plan) => !plan.active), [plans]);
    const validatePlanForm = useCallback((draft: PlanFormState) => {
        if (!draft.label.trim()) {
            return t('admin_plans_validation_label');
        }
        if (!isWholeNumber(draft.days) || Number(draft.days) <= 0) {
            return t('admin_plans_validation_days');
        }
        if (!isWholeNumber(draft.price) || Number(draft.price) <= 0) {
            return t('admin_plans_validation_price');
        }
        if (!isWholeNumber(draft.trafficLimitGB)) {
            return t('admin_plans_validation_traffic');
        }
        const wholesaleTrimmed = draft.wholesalePrice.trim();
        if (wholesaleTrimmed) {
            if (!isWholeNumber(wholesaleTrimmed) || Number(wholesaleTrimmed) <= 0) {
                return t('admin_plans_validation_wholesale');
            }
            if (Number(wholesaleTrimmed) > Number(draft.price)) {
                return t('admin_plans_validation_wholesale_max');
            }
        }
        return null;
    }, [t]);
    const createValidationError = validatePlanForm(form);

    const handleCreatePlan = async (event: FormEvent<HTMLFormElement>) => {
        event.preventDefault();
        if (!initData || submitting) return;
        if (createValidationError) {
            setActionError(createValidationError);
            setActionSuccess(null);
            return;
        }

        setSubmitting(true);
        setActionError(null);
        setActionSuccess(null);

        try {
            const nextSortOrder = plans.reduce((max, plan) => Math.max(max, plan.sort_order), -1) + 1;
            const response = await fetchWithTelegramAuth('/api/admin/plans', initData, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({
                    label: form.label.trim(),
                    days: Number(form.days),
                    price: Number(form.price),
                    traffic_limit_gb: Number(form.trafficLimitGB),
                    sort_order: nextSortOrder,
                    wholesale_price: parseWholesalePrice(form.wholesalePrice),
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
                setActionError(message || t('admin_plans_create_error'));
                return;
            }

            const createdPlan = await response.json() as AdminPlan;
            setPlans((prev) => [...prev, createdPlan].sort((a, b) => a.sort_order - b.sort_order));
            setEditForms((prev) => ({ ...prev, [createdPlan.id]: formFromPlan(createdPlan) }));
            setForm(INITIAL_FORM);
            setActionSuccess(t('admin_plans_create_success'));
        } catch {
            setActionError(t('admin_plans_create_error'));
        } finally {
            setSubmitting(false);
        }
    };

    const handleSavePlan = async (plan: AdminPlan) => {
        if (!initData || savingId === plan.id) return;

        const draft = editForms[plan.id] ?? formFromPlan(plan);
        const validationError = validatePlanForm(draft);
        if (validationError) {
            setActionError(validationError);
            setActionSuccess(null);
            return;
        }
        setSavingId(plan.id);
        setActionError(null);
        setActionSuccess(null);

        try {
            const response = await fetchWithTelegramAuth(`/api/admin/plans/${encodeURIComponent(plan.id)}`, initData, {
                method: 'PATCH',
                headers: {
                    'Content-Type': 'application/json',
                },
                body: JSON.stringify({
                    label: draft.label.trim(),
                    days: Number(draft.days),
                    price: Number(draft.price),
                    traffic_limit_gb: Number(draft.trafficLimitGB),
                    sort_order: plan.sort_order,
                    wholesale_price: parseWholesalePrice(draft.wholesalePrice),
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
                setActionError(message || t('admin_plans_update_error'));
                return;
            }

            const updatedPlan = await response.json() as AdminPlan;
            setPlans((prev) => prev.map((item) => item.id === updatedPlan.id ? updatedPlan : item).sort((a, b) => a.sort_order - b.sort_order));
            setEditForms((prev) => ({ ...prev, [updatedPlan.id]: formFromPlan(updatedPlan) }));
            setActionSuccess(t('admin_plans_update_success'));
        } catch {
            setActionError(t('admin_plans_update_error'));
        } finally {
            setSavingId(null);
        }
    };

    const handleArchivePlan = async (plan: AdminPlan) => {
        if (!initData || archivingId === plan.id) return;
        if (!window.confirm(t('admin_plans_archive_confirm').replace('{{label}}', plan.label))) return;

        setArchivingId(plan.id);
        setActionError(null);
        setActionSuccess(null);

        try {
            const response = await fetchWithTelegramAuth(`/api/admin/plans/${encodeURIComponent(plan.id)}`, initData, {
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
                setActionError(message || t('admin_plans_archive_error'));
                return;
            }

            setPlans((prev) => prev.map((item) => item.id === plan.id ? { ...item, active: false } : item));
            setActionSuccess(t('admin_plans_archive_success'));
        } catch {
            setActionError(t('admin_plans_archive_error'));
        } finally {
            setArchivingId(null);
        }
    };

    if (loading) return <LoadingScreen message={t('admin_plans_loading')} />;
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
        return <ErrorScreen message={t('admin_plans_forbidden')} />;
    }
    if (error) {
        return (
            <ErrorScreen
                message={error}
                onRetry={() => { void loadPlans(); }}
                retryLabel={t('retry')}
            />
        );
    }

    return (
        <div className="animate-fade-in admin-promo-shell">
            <section className="digital-card admin-promo-hero">
                <div style={{ display: 'grid', gap: 14 }}>
                    <div>
                        <h1 style={{ fontSize: 22, fontWeight: 800, margin: 0, color: 'var(--digital-card-text)' }}>{t('admin_plans_title')}</h1>
                        <p style={{ fontSize: 13, lineHeight: 1.5, margin: '8px 0 0', color: 'var(--digital-card-hint)' }}>
                            {t('admin_plans_subtitle')}
                        </p>
                    </div>
                    <div className="admin-promo-stat-grid">
                        <div className="admin-promo-stat-card">
                            <span className="admin-promo-stat-label">{t('admin_plans_total_label')}</span>
                            <strong className="admin-promo-stat-value">{plans.length}</strong>
                        </div>
                        <div className="admin-promo-stat-card">
                            <span className="admin-promo-stat-label">{t('admin_plans_active_label')}</span>
                            <strong className="admin-promo-stat-value">{activePlans.length}</strong>
                        </div>
                        <div className="admin-promo-stat-card">
                            <span className="admin-promo-stat-label">{t('admin_plans_archived_label')}</span>
                            <strong className="admin-promo-stat-value">{archivedPlans.length}</strong>
                        </div>
                    </div>
                </div>
            </section>

            <section className="glass-card admin-promo-panel">
                <div style={{ display: 'grid', gap: 6, marginBottom: 18 }}>
                    <h2 style={{ fontSize: 16, margin: 0 }}>{t('admin_plans_create_title')}</h2>
                    <p className="text-hint" style={{ fontSize: 12, margin: 0 }}>{t('admin_plans_form_caption')}</p>
                </div>

                <form className="admin-promo-form" onSubmit={(event) => { playClick(); void handleCreatePlan(event); }}>
                    <label className="admin-promo-field admin-promo-field-full">
                        <span className="admin-promo-label">{t('admin_plans_label_label')}</span>
                        <input
                            className="admin-promo-input"
                            value={form.label}
                            onChange={(event) => setForm((prev) => ({ ...prev, label: event.target.value }))}
                            placeholder={t('admin_plans_label_placeholder')}
                            aria-label={t('admin_plans_label_label')}
                            required
                        />
                    </label>
                    <label className="admin-promo-field">
                        <span className="admin-promo-label">{t('admin_plans_days_label')}</span>
                        <input
                            className="admin-promo-input"
                            type="number"
                            min="1"
                            value={form.days}
                            onChange={(event) => setForm((prev) => ({ ...prev, days: event.target.value }))}
                            aria-label={t('admin_plans_days_label')}
                            required
                        />
                    </label>
                    <label className="admin-promo-field">
                        <span className="admin-promo-label">{t('admin_plans_price_label')}</span>
                        <input
                            className="admin-promo-input"
                            type="number"
                            min="1"
                            value={form.price}
                            onChange={(event) => setForm((prev) => ({ ...prev, price: event.target.value }))}
                            aria-label={t('admin_plans_price_label')}
                            required
                        />
                    </label>
                    <label className="admin-promo-field admin-promo-field-full">
                        <span className="admin-promo-label">{t('admin_plans_traffic_label')}</span>
                        <input
                            className="admin-promo-input"
                            type="number"
                            min="0"
                            value={form.trafficLimitGB}
                            onChange={(event) => setForm((prev) => ({ ...prev, trafficLimitGB: event.target.value }))}
                            aria-label={t('admin_plans_traffic_label')}
                            required
                        />
                    </label>
                    <label className="admin-promo-field admin-promo-field-full">
                        <span className="admin-promo-label">{t('admin_plans_wholesale_label')}</span>
                        <input
                            className="admin-promo-input"
                            type="number"
                            min="1"
                            value={form.wholesalePrice}
                            onChange={(event) => setForm((prev) => ({ ...prev, wholesalePrice: event.target.value }))}
                            aria-label={t('admin_plans_wholesale_label')}
                            placeholder={t('admin_plans_wholesale_placeholder')}
                        />
                    </label>
                    <button className="btn-primary admin-promo-submit" type="submit" disabled={submitting || createValidationError !== null}>
                        {submitting ? t('admin_plans_creating') : t('admin_plans_create')}
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

            <section className="glass-card admin-promo-panel">
                <div style={{ display: 'grid', gap: 6, marginBottom: 18 }}>
                    <h2 style={{ fontSize: 16, margin: 0 }}>{t('admin_plans_list_title')}</h2>
                    <p className="text-hint" style={{ fontSize: 12, margin: 0 }}>{t('admin_plans_list_subtitle')}</p>
                </div>

                {plans.length === 0 ? (
                    <div className="glass-card" style={{ padding: 18, display: 'grid', gap: 6 }}>
                        <strong>{t('admin_plans_empty')}</strong>
                        <span className="text-hint" style={{ fontSize: 12 }}>{t('admin_plans_empty_detail')}</span>
                    </div>
                ) : (
                    <div style={{ display: 'grid', gap: 12 }}>
                        {plans.map((plan) => {
                            const draft = editForms[plan.id] ?? formFromPlan(plan);
                            const validationError = validatePlanForm(draft);
                            const canArchive = plan.active && activePlans.length > 1;
                            const tone = plan.active
                                ? { background: 'rgba(52, 199, 89, 0.12)', color: 'var(--color-success)', border: 'rgba(52, 199, 89, 0.22)' }
                                : { background: 'rgba(255, 59, 48, 0.1)', color: 'var(--color-danger)', border: 'rgba(255, 59, 48, 0.18)' };

                            return (
                                <div key={plan.id} data-testid={`admin-plan-${plan.id}`} className="glass-card admin-promo-list-card">
                                    <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, alignItems: 'flex-start' }}>
                                        <div>
                                            <h3 style={{ margin: 0, fontSize: 16 }}>{draft.label || plan.label}</h3>
                                            <div className="text-hint" style={{ fontSize: 12, marginTop: 4 }}>
                                                {plan.active ? t('admin_plans_status_active') : t('admin_plans_status_archived')}
                                            </div>
                                        </div>
                                        <div style={{
                                            padding: '6px 10px',
                                            borderRadius: 999,
                                            border: `1px solid ${tone.border}`,
                                            background: tone.background,
                                            color: tone.color,
                                            fontSize: 11,
                                            fontWeight: 700,
                                        }}>
                                            {plan.active ? t('admin_plans_status_active') : t('admin_plans_status_archived')}
                                        </div>
                                    </div>

                                    <div className="admin-promo-form" style={{ marginTop: 16 }}>
                                        <label className="admin-promo-field admin-promo-field-full">
                                            <span className="admin-promo-label">{t('admin_plans_label_label')}</span>
                                            <input
                                                className="admin-promo-input"
                                                aria-label={t('admin_plans_label_label')}
                                                value={draft.label}
                                                onChange={(event) => setEditForms((prev) => ({ ...prev, [plan.id]: { ...draft, label: event.target.value } }))}
                                                required
                                            />
                                        </label>
                                        <label className="admin-promo-field">
                                            <span className="admin-promo-label">{t('admin_plans_days_label')}</span>
                                            <input
                                                className="admin-promo-input"
                                                type="number"
                                                min="1"
                                                aria-label={t('admin_plans_days_label')}
                                                value={draft.days}
                                                onChange={(event) => setEditForms((prev) => ({ ...prev, [plan.id]: { ...draft, days: event.target.value } }))}
                                                required
                                            />
                                        </label>
                                        <label className="admin-promo-field">
                                            <span className="admin-promo-label">{t('admin_plans_price_label')}</span>
                                            <input
                                                className="admin-promo-input"
                                                type="number"
                                                min="1"
                                                aria-label={t('admin_plans_price_label')}
                                                value={draft.price}
                                                onChange={(event) => setEditForms((prev) => ({ ...prev, [plan.id]: { ...draft, price: event.target.value } }))}
                                                required
                                            />
                                        </label>
                                        <label className="admin-promo-field admin-promo-field-full">
                                            <span className="admin-promo-label">{t('admin_plans_traffic_label')}</span>
                                            <input
                                                className="admin-promo-input"
                                                type="number"
                                                min="0"
                                                aria-label={t('admin_plans_traffic_label')}
                                                value={draft.trafficLimitGB}
                                                onChange={(event) => setEditForms((prev) => ({ ...prev, [plan.id]: { ...draft, trafficLimitGB: event.target.value } }))}
                                                required
                                            />
                                        </label>
                                        <label className="admin-promo-field admin-promo-field-full">
                                            <span className="admin-promo-label">{t('admin_plans_wholesale_label')}</span>
                                            <input
                                                className="admin-promo-input"
                                                type="number"
                                                min="1"
                                                aria-label={t('admin_plans_wholesale_label')}
                                                value={draft.wholesalePrice}
                                                onChange={(event) => setEditForms((prev) => ({ ...prev, [plan.id]: { ...draft, wholesalePrice: event.target.value } }))}
                                                placeholder={t('admin_plans_wholesale_placeholder')}
                                            />
                                        </label>
                                    </div>

                                    <div className="admin-promo-metrics" style={{ marginTop: 16 }}>
                                        <div className="admin-promo-metric-card">
                                            <span className="admin-promo-metric-label">{t('admin_plans_days_metric')}</span>
                                            <strong className="admin-promo-metric-value">{plan.days}</strong>
                                        </div>
                                        <div className="admin-promo-metric-card">
                                            <span className="admin-promo-metric-label">{t('admin_plans_price_metric')}</span>
                                            <strong className="admin-promo-metric-value">{plan.price.toLocaleString()} {plan.currency}</strong>
                                        </div>
                                        <div className="admin-promo-metric-card">
                                            <span className="admin-promo-metric-label">{t('admin_plans_wholesale_metric')}</span>
                                            <strong className="admin-promo-metric-value">
                                                {plan.wholesale_price != null
                                                    ? `${plan.wholesale_price.toLocaleString()} ${plan.currency}`
                                                    : t('admin_plans_wholesale_none')}
                                            </strong>
                                        </div>
                                        <div className="admin-promo-metric-card">
                                            <span className="admin-promo-metric-label">{t('admin_plans_traffic_metric')}</span>
                                            <strong className="admin-promo-metric-value">{plan.traffic_limit_gb === 0 ? t('unlimited') : `${plan.traffic_limit_gb} GB`}</strong>
                                        </div>
                                    </div>

                                    <div style={{ display: 'flex', gap: 10, marginTop: 16, justifyContent: 'flex-end' }}>
                                        <button
                                            type="button"
                                            className="btn-secondary"
                                            onClick={() => { playClick(); void handleSavePlan(plan); }}
                                            disabled={savingId === plan.id || validationError !== null}
                                        >
                                            {savingId === plan.id ? t('admin_plans_saving') : t('admin_plans_save')}
                                        </button>
                                        {plan.active && (
                                            <button
                                                type="button"
                                                className="btn-danger"
                                                onClick={() => { playClick(); void handleArchivePlan(plan); }}
                                                disabled={archivingId === plan.id || !canArchive}
                                                title={!canArchive ? t('admin_plans_keep_one_active') : undefined}
                                            >
                                                {archivingId === plan.id ? t('admin_plans_archiving') : t('admin_plans_archive')}
                                            </button>
                                        )}
                                    </div>
                                </div>
                            );
                        })}
                    </div>
                )}
            </section>
        </div>
    );
}
