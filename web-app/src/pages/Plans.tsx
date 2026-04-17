import { useEffect, useState, useCallback, useRef } from 'react';
import { useTelegram } from '../lib/twa';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { useLanguage } from '../lib/LanguageContext';
import { LoadingScreen } from '../components/LoadingScreen';
import { ErrorScreen } from '../components/ErrorScreen';
import { SessionExpiredScreen } from '../components/SessionExpiredScreen';
import { TipBox } from '../components/TipBox';
import { Plan, UserData } from '../lib/types';
import { useMXBrownSound } from '../lib/useMXBrownSound';
import { APIError, fetchJSON, isAPIStatus } from '../lib/http';
import { clearTelegramSession, fetchUserScopedJSONWithTelegramAuth, fetchWithTelegramAuth } from '../lib/auth';
import { getVisiblePlans } from '../lib/plans';


export function Plans() {
    const { tg, initData, close } = useTelegram();
    const { t, language } = useLanguage();
    const navigate = useNavigate();
    const { playClick } = useMXBrownSound();
    const [searchParams] = useSearchParams();
    const [plans, setPlans] = useState<Plan[]>([]);
    const [userData, setUserData] = useState<UserData | null>(null);
    const [loading, setLoading] = useState(true);
    const [promoCode, setPromoCode] = useState('');
    const [promoStatus, setPromoStatus] = useState<'idle' | 'validating' | 'valid' | 'invalid'>('idle');
    const [discountPercent, setDiscountPercent] = useState<number>(0);
    const [appliedPromoCode, setAppliedPromoCode] = useState('');
    const [loadError, setLoadError] = useState<string | null>(null);
    const [promoError, setPromoError] = useState<string | null>(null);
    const [authExpired, setAuthExpired] = useState(false);
    const promoRequestRef = useRef(0);

    const extendKeyId = searchParams.get('extend');
    const isExtend = !!extendKeyId;
    const isWalletTopup = searchParams.get('walletTopup') === 'true';

    const extendingKey = userData?.keys?.find(k => k.id === Number(extendKeyId));
    const currentExpiry = extendingKey?.expire_at ? new Date(extendingKey.expire_at) : null;

    const handleBack = useCallback(() => {
        navigate(isWalletTopup ? '/wallet' : '/');
    }, [navigate, isWalletTopup]);

    useEffect(() => {
        if (!tg) return;
        tg.BackButton.show();
        tg.BackButton.onClick(handleBack);
        return () => tg.BackButton.offClick(handleBack);
    }, [tg, handleBack]);

    const loadPlans = useCallback(async () => {
        if (!initData) {
            setLoading(false);
            return;
        }

        setLoading(true);
        setLoadError(null);
        setAuthExpired(false);

        try {
            const plansData = await fetchJSON<Plan[]>('/api/plans');
            setPlans(getVisiblePlans(Array.isArray(plansData) ? plansData : []));

            if (isWalletTopup) {
                setUserData(null);
                return;
            }

            const meData = await fetchUserScopedJSONWithTelegramAuth<UserData>(
                '/api/me',
                initData,
                tg?.initDataUnsafe?.user?.id,
            );
            setUserData(meData);
        } catch (err) {
            console.warn('Plans load error:', err);
            if (isAPIStatus(err, 401)) {
                clearTelegramSession();
                setAuthExpired(true);
                return;
            }
            if (err instanceof APIError && err.body) {
                setLoadError(err.body);
                return;
            }
            setLoadError(t('plans_load_error'));
        } finally {
            setLoading(false);
        }
    }, [initData, isWalletTopup, t, tg]);

    useEffect(() => {
        void loadPlans();
    }, [loadPlans]);

    const handleApplyPromo = async () => {
        const normalizedCode = promoCode.trim();
        if (!normalizedCode || !initData) return;

        const requestId = promoRequestRef.current + 1;
        promoRequestRef.current = requestId;
        setPromoStatus('validating');
        setPromoError(null);

        try {
            const res = await fetchWithTelegramAuth(`/api/promo/validate?code=${encodeURIComponent(normalizedCode)}`, initData);
            if (promoRequestRef.current !== requestId) return;

            if (res.ok) {
                const data = await res.json();
                if (data.valid) {
                    setPromoStatus('valid');
                    setDiscountPercent(data.discount_percent);
                    setAppliedPromoCode(data.code);
                } else {
                    setPromoStatus('invalid');
                    setDiscountPercent(0);
                    setAppliedPromoCode('');
                }
                return;
            }

            if (res.status === 404) {
                setPromoStatus('invalid');
                setDiscountPercent(0);
                setAppliedPromoCode('');
                return;
            }

            if (res.status === 401) {
                clearTelegramSession();
                setAuthExpired(true);
                setPromoStatus('idle');
                return;
            }

            setPromoStatus('idle');
            setDiscountPercent(0);
            setAppliedPromoCode('');
            setPromoError(t('promo_service_unavailable'));
        } catch {
            if (promoRequestRef.current !== requestId) return;
            setPromoStatus('idle');
            setDiscountPercent(0);
            setAppliedPromoCode('');
            setPromoError(t('promo_service_unavailable'));
        }
    };

    const handleClearPromo = () => {
        setPromoCode('');
        setPromoStatus('idle');
        setPromoError(null);
        setDiscountPercent(0);
        setAppliedPromoCode('');
        promoRequestRef.current += 1;
    };

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
    if (loading) return <LoadingScreen message={t('loading_plans')} />;
    if (loadError) {
        return (
            <ErrorScreen
                message={loadError}
                onRetry={() => { void loadPlans(); }}
                retryLabel={t('retry')}
            />
        );
    }

    const calcNewExpiry = (days: number) => {
        const base = currentExpiry && currentExpiry > new Date() ? currentExpiry : new Date();
        const d = new Date(base);
        d.setDate(d.getDate() + days);
        return d.toLocaleDateString(language === 'en' ? 'en-US' : 'my-MM', { year: 'numeric', month: 'short', day: 'numeric' });
    };

    const TOPUP_AMOUNTS = [5000, 10000, 30000, 50000, 100000];

    // Extend plan filtering rules:
    //   Key is ACTIVE  → lock to same traffic type (unlimited↔limited, no switching)
    //   Key is EXPIRED → all plans available (starting fresh)
    //   New purchase   → all plans available
    const extendingKeyIsActive = extendingKey?.status === 'active';
    const extendingKeyIsUnlimited = extendingKey?.traffic_limit_gb === 0;
    const filteredPlans = isExtend && extendingKey && extendingKeyIsActive
        ? plans.filter(p =>
            extendingKeyIsUnlimited
                ? p.traffic_limit_gb === 0   // Unlimited → Unlimited only
                : p.traffic_limit_gb > 0     // Limited   → Limited only
        )
        : plans;


    const itemsToDisplay = isWalletTopup
        ? TOPUP_AMOUNTS.map(amount => ({
            id: `topup-${amount}`,
            label: `${amount.toLocaleString()} ${plans[0]?.currency || 'MMK'}`,
            days: 0,
            price: amount,
            traffic_limit_gb: 0,
            currency: plans[0]?.currency || 'MMK',
            isTopUp: true
        }))
        : filteredPlans;

    const displayItems = itemsToDisplay.map((item: Plan & { isTopUp?: boolean; discountedPrice?: number }) => {
        if (!isWalletTopup && discountPercent > 0) {
            return { ...item, discountedPrice: Math.round(item.price * (1 - discountPercent / 100)) };
        }
        return item;
    });
    const showEmptyPlans = !isWalletTopup && displayItems.length === 0;

    const bestIdx = !isWalletTopup && displayItems.length > 0
        ? displayItems.reduce((b, p, i) => {
            const priceP = (p as Plan & { discountedPrice?: number }).discountedPrice || p.price;
            const priceB = (displayItems[b] as Plan & { discountedPrice?: number }).discountedPrice || displayItems[b].price;
            const daysP = p.days || 1;
            const daysB = displayItems[b].days || 1;
            return (priceP / daysP) < (priceB / daysB) ? i : b;
        }, 0)
        : -1;

    return (
        <div className="animate-fade-in" style={{ padding: 16, display: 'flex', flexDirection: 'column', gap: 16, minHeight: '100vh' }}>
            {/* Header */}
            <div style={{ textAlign: 'center', padding: '8px 0' }}>
                <h1 style={{ fontSize: 20, fontWeight: 700, margin: 0 }}>
                    {isWalletTopup ? t('title_top_up') : (isExtend ? t('title_extend') : t('title_choose_plan'))}
                </h1>
                {isExtend && extendingKey && !isWalletTopup && (
                    <p className="text-hint" style={{ fontSize: 12, margin: '6px 0 0' }}>
                        {t('subtitle_extending', { label: extendingKey.label })}
                        {currentExpiry && <> · {t('expires_on', { date: currentExpiry.toLocaleDateString(language === 'en' ? 'en-US' : 'my-MM', { month: 'short', day: 'numeric' }) })}</>}
                    </p>
                )}
                {!isExtend && !isWalletTopup && (
                    <div className="text-hint" style={{ fontSize: 11, marginTop: 4, opacity: 0.7 }}>
                        {t('subtitle_new_key_hint')}
                    </div>
                )}
            </div>

            {/* Promo Code Input */}
            {!isWalletTopup && (
                <div className="glass-card" style={{ padding: 14, display: 'grid', gap: 12 }}>
                    <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', gap: 12 }}>
                        <div>
                            <div style={{ fontSize: 14, fontWeight: 700 }}>{t('promo_title')}</div>
                            <div className="text-hint" style={{ fontSize: 12, marginTop: 3 }}>{t('promo_subtitle')}</div>
                        </div>
                        {promoStatus === 'valid' && appliedPromoCode && (
                            <div
                                style={{
                                    background: 'rgba(16, 185, 129, 0.14)',
                                    color: 'var(--color-success)',
                                    border: '1px solid rgba(16, 185, 129, 0.28)',
                                    borderRadius: 999,
                                    padding: '5px 10px',
                                    fontSize: 11,
                                    fontWeight: 700,
                                    letterSpacing: 0.2,
                                    whiteSpace: 'nowrap',
                                }}
                            >
                                {appliedPromoCode}
                            </div>
                        )}
                    </div>

                    <div style={{ display: 'flex', gap: 8, alignItems: 'center' }}>
                        <input
                            type="text"
                            value={promoCode}
                            onChange={(e) => {
                                const next = e.target.value;
                                setPromoCode(next);
                                if (next !== appliedPromoCode) {
                                    setPromoStatus('idle');
                                    setPromoError(null);
                                    setDiscountPercent(0);
                                    setAppliedPromoCode('');
                                    promoRequestRef.current += 1;
                                }
                            }}
                            onKeyDown={(e) => {
                                if (e.key === 'Enter' && promoCode.trim() && promoStatus !== 'validating') {
                                    e.preventDefault();
                                    playClick();
                                    void handleApplyPromo();
                                }
                            }}
                            placeholder={t('promo_placeholder')}
                            aria-label={t('promo_placeholder')}
                            aria-invalid={promoStatus === 'invalid' || promoError !== null}
                            aria-describedby={(promoStatus === 'valid' || promoStatus === 'invalid' || promoError) ? 'promo-feedback' : undefined}
                            style={{
                                flex: 1,
                                background: 'var(--input-bg)',
                                border: '1px solid var(--input-border)',
                                borderRadius: 10,
                                padding: '11px 12px',
                                color: 'var(--tg-text)',
                                fontSize: 14,
                                letterSpacing: promoCode ? 0.4 : 0,
                            }}
                        />
                        <button
                            onClick={() => { playClick(); void handleApplyPromo(); }}
                            disabled={promoStatus === 'validating' || !promoCode.trim()}
                            className="btn-secondary"
                            style={{
                                padding: '11px 16px',
                                fontSize: 13,
                                opacity: !promoCode.trim() ? 0.5 : 1
                            }}
                        >
                            {promoStatus === 'validating' ? t('promo_validating') : t('promo_apply')}
                        </button>
                    </div>

                    {(promoCode || appliedPromoCode) && promoStatus !== 'validating' && (
                        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', gap: 8 }}>
                            <div className="text-hint" style={{ fontSize: 11 }}>
                                {discountPercent > 0
                                    ? t('promo_discount_ready', { percent: String(discountPercent) })
                                    : t('promo_subtitle')}
                            </div>
                            <button
                                type="button"
                                className="btn-secondary"
                                onClick={() => { playClick(); handleClearPromo(); }}
                                style={{ padding: '7px 12px', fontSize: 12, height: 'auto' }}
                            >
                                {t('promo_clear')}
                            </button>
                        </div>
                    )}
                </div>
            )}
            {!isWalletTopup && promoStatus === 'valid' && (
                <div id="promo-feedback" role="status" style={{ color: 'var(--color-success)', fontSize: 12, marginTop: -8, marginLeft: 4 }}>
                    {t('promo_valid', { percent: String(discountPercent) })}
                </div>
            )}
            {!isWalletTopup && promoStatus === 'invalid' && (
                <div id="promo-feedback" role="alert" style={{ color: 'var(--color-danger)', fontSize: 12, marginTop: -8, marginLeft: 4 }}>
                    {t('promo_invalid')}
                </div>
            )}
            {!isWalletTopup && promoError && (
                <div id="promo-feedback" role="alert" style={{ color: 'var(--color-danger)', fontSize: 12, marginTop: -8, marginLeft: 4 }}>
                    {promoError}
                </div>
            )}

            {/* Extend info */}
            {isExtend && !isWalletTopup && (
                <TipBox variant="info" icon="ℹ️">{t('help_extend_info')}</TipBox>
            )}

            {/* New key info */}
            {!isExtend && !isWalletTopup && (
                <TipBox variant="info" icon="ℹ️">{t('help_new_key_info')}</TipBox>
            )}

            {/* Plan / Top-up cards */}
            {showEmptyPlans ? (
                <div className="glass-card" style={{ padding: 18, display: 'grid', gap: 8 }}>
                    <strong>{isExtend ? t('plans_empty_extend_title') : t('plans_empty_title')}</strong>
                    <span className="text-hint" style={{ fontSize: 12 }}>
                        {isExtend ? t('plans_empty_extend_desc') : t('plans_empty_desc')}
                    </span>
                </div>
            ) : (
                <div className="stagger" style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
                    {displayItems.map((item: Plan & { isTopUp?: boolean; discountedPrice?: number }, idx: number) => {
                    const price = item.discountedPrice || item.price;
                    const hasDiscount = item.discountedPrice !== undefined && item.discountedPrice < item.price;
                    const checkoutParams = new URLSearchParams();

                    if (isExtend && extendKeyId) {
                        checkoutParams.set('extend', extendKeyId);
                    }
                    if (isWalletTopup) {
                        checkoutParams.set('walletTopup', 'true');
                        checkoutParams.set('amount', String(item.price));
                    }
                    if (!isWalletTopup && appliedPromoCode) {
                        checkoutParams.set('promo', appliedPromoCode);
                        checkoutParams.set('discount', String(discountPercent));
                    }

                    const checkoutPath = isWalletTopup ? '/checkout' : `/checkout/${item.id}`;
                    const checkoutUrl = `${checkoutPath}${checkoutParams.toString() ? `?${checkoutParams.toString()}` : ''}`;

                    return (
                        <Link
                            key={idx}
                            to={checkoutUrl}
                            style={{ textDecoration: 'none', color: 'inherit' }}
                        >
                            <div
                                className={`glass-card ${idx === bestIdx && !isWalletTopup ? 'glass-card-active' : ''}`}
                                style={{ padding: 16, position: 'relative', transition: 'transform 0.15s ease' }}
                            >
                                {idx === bestIdx && !isWalletTopup && (
                                    <div style={{
                                        position: 'absolute', top: -8, right: 12,
                                        background: 'linear-gradient(135deg, #5ebbff, #007AFF)',
                                        color: '#fff', fontSize: 10, fontWeight: 700,
                                        padding: '3px 10px', borderRadius: 20,
                                        textTransform: 'uppercase', letterSpacing: 0.5,
                                    }}>
                                        {t('best_value')}
                                    </div>
                                )}

                                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                                    <div>
                                        <div style={{ fontWeight: 600, fontSize: 15 }}>{item.label}</div>
                                        {!isWalletTopup && (
                                            <div className="text-hint" style={{ fontSize: 12, marginTop: 3, display: 'flex', alignItems: 'center', gap: 6 }}>
                                                <span>📅 {item.days} {t('days_left').replace(' left', '')}</span>
                                                <span style={{ opacity: 0.3 }}>·</span>
                                                <span style={item.traffic_limit_gb === 0 ? { color: 'var(--color-success)' } : {}}>
                                                    {item.traffic_limit_gb > 0 ? `📊 ${item.traffic_limit_gb} GB` : t('unlimited')}
                                                </span>
                                            </div>
                                        )}
                                        {isExtend && !isWalletTopup && (
                                            <div className="text-hint" style={{ fontSize: 10, marginTop: 4, color: 'var(--color-success)' }}>
                                                {t('new_expiry', { date: calcNewExpiry(item.days) })}
                                            </div>
                                        )}
                                        {isWalletTopup && (
                                            <div className="text-hint" style={{ fontSize: 12, marginTop: 3 }}>
                                                {t('top_up_amount', { amount: item.price.toLocaleString(), currency: item.currency })}
                                            </div>
                                        )}
                                    </div>
                                    <div style={{ textAlign: 'right' }}>
                                        <div className="text-link" style={{ fontSize: 18, fontWeight: 700, color: hasDiscount ? 'var(--color-success)' : undefined }}>
                                            {price.toLocaleString()}
                                        </div>
                                        {hasDiscount && (
                                            <div style={{ fontSize: 13, textDecoration: 'line-through', opacity: 0.5 }}>
                                                {item.price.toLocaleString()}
                                            </div>
                                        )}
                                        <div className="text-hint" style={{ fontSize: 11 }}>{item.currency}</div>
                                        {!isWalletTopup && (
                                            <div className="text-hint" style={{ fontSize: 9, marginTop: 2 }}>
                                                {Math.round(price / item.days)} {t('per_day', { currency: item.currency.toLowerCase() })}
                                            </div>
                                        )}
                                    </div>
                                </div>
                            </div>
                        </Link>
                    );
                    })}
                </div>
            )}

            <TipBox variant="success" icon="✅">{t('help_payments')}</TipBox>
        </div>
    );
}
