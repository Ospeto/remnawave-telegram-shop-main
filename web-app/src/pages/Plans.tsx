import { useEffect, useState } from 'react';
import { useTelegram } from '../lib/twa';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { useLanguage } from '../lib/LanguageContext';

interface Plan {
    label: string;
    days: number;
    price: number;
    traffic_limit_gb: number;
    currency: string;
}

interface DisplayPlan extends Plan {
    discountedPrice?: number;
}

interface UserData {
    is_active: boolean;
    expire_at: string | null;
    days_remaining: number;
    keys: { id: number; label: string; expire_at: string | null; status: string; traffic_limit_gb: number }[];
}

export function Plans() {
    const { tg, initData } = useTelegram();
    const { t, language } = useLanguage();
    const navigate = useNavigate();
    const [searchParams] = useSearchParams();
    const [plans, setPlans] = useState<Plan[]>([]);
    const [userData, setUserData] = useState<UserData | null>(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [promoCode, setPromoCode] = useState('');
    const [promoStatus, setPromoStatus] = useState<'idle' | 'validating' | 'valid' | 'invalid'>('idle');
    const [discountPercent, setDiscountPercent] = useState<number>(0);
    const [appliedPromoCode, setAppliedPromoCode] = useState('');

    const extendKeyId = searchParams.get('extend');
    const isExtend = !!extendKeyId;

    // Find the key being extended
    const extendingKey = userData?.keys?.find(k => k.id === Number(extendKeyId));
    const currentExpiry = extendingKey?.expire_at ? new Date(extendingKey.expire_at) : null;

    useEffect(() => {
        if (tg) {
            tg.BackButton.show();
            const handler = () => navigate('/');
            tg.BackButton.onClick(handler);
            return () => tg.BackButton.offClick(handler);
        }
    }, [tg, navigate]);

    useEffect(() => {
        if (!initData) return;
        const headers = { 'Authorization': `twa ${initData}` };
        Promise.all([
            fetch('/api/plans', { headers }).then(r => r.json()),
            fetch('/api/me', { headers }).then(r => r.json()),
        ])
            .then(([p, m]) => { setPlans(p || []); setUserData(m); })
            .catch(err => {
                setError(`${err.name}: ${err.message}`);
            })
            .finally(() => setLoading(false));
    }, [initData]);

    const handleApplyPromo = () => {
        if (!promoCode.trim()) return;
        setPromoStatus('validating');
        fetch(`/api/promo/validate?code=${encodeURIComponent(promoCode)}`, {
            headers: { 'Authorization': `twa ${initData}` }
        })
            .then(async res => {
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
                } else {
                    setPromoStatus('invalid');
                    setDiscountPercent(0);
                    setAppliedPromoCode('');
                }
            })
            .catch(() => {
                setPromoStatus('invalid');
                setDiscountPercent(0);
                setAppliedPromoCode('');
            });
    };

    if (loading) return (
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: '100vh', gap: 12 }}>
            <div className="spinner" />
            <span className="text-hint" style={{ fontSize: 13 }}>{t('loading_plans')}</span>
        </div>
    );

    if (error) return (
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: '100vh', gap: 16, padding: 24 }}>
            <div style={{ fontSize: 48 }}>⚠️</div>
            <p style={{ color: '#ff3b30' }}>{t('error_prefix')} {error}</p>
            <button className="btn-secondary" onClick={() => window.location.reload()}>{t('retry')}</button>
        </div>
    );

    const calcNewExpiry = (days: number) => {
        const base = currentExpiry && currentExpiry > new Date() ? currentExpiry : new Date();
        const d = new Date(base);
        d.setDate(d.getDate() + days);
        return d.toLocaleDateString(language === 'en' ? 'en-US' : 'my-MM', { year: 'numeric', month: 'short', day: 'numeric' });
    };

    // Apply discount if valid promo
    const displayPlans: DisplayPlan[] = plans.map(p => {
        if (discountPercent > 0) {
            return { ...p, discountedPrice: Math.round(p.price * (1 - discountPercent / 100)) };
        }
        return p;
    });

    // Best value = lowest price per day
    const bestIdx = displayPlans.length > 0
        ? displayPlans.reduce((b, p, i) => {
            const priceP = p.discountedPrice ?? p.price;
            const priceB = displayPlans[b].discountedPrice ?? displayPlans[b].price;
            return (priceP / p.days) < (priceB / displayPlans[b].days) ? i : b;
        }, 0)
        : -1;

    return (
        <div className="animate-fade-in" style={{ padding: 16, display: 'flex', flexDirection: 'column', gap: 16, minHeight: '100vh' }}>
            {/* Header */}
            <div style={{ textAlign: 'center', padding: '8px 0' }}>
                <h1 style={{ fontSize: 20, fontWeight: 700, margin: 0 }}>
                    {isExtend ? t('title_extend') : t('title_choose_plan')}
                </h1>
                {isExtend && extendingKey && (
                    <p className="text-hint" style={{ fontSize: 12, margin: '6px 0 0' }}>
                        {t('subtitle_extending', { label: extendingKey.label })}
                        {currentExpiry && <> · {t('expires_on', { date: currentExpiry.toLocaleDateString(language === 'en' ? 'en-US' : 'my-MM', { month: 'short', day: 'numeric' }) })}</>}
                    </p>
                )}
                {!isExtend && (
                    <p className="text-hint" style={{ fontSize: 12, margin: '6px 0 0' }}>
                        {t('subtitle_new_key')}
                    </p>
                )}
                {!isExtend && (
                    <div className="text-hint" style={{ fontSize: 11, marginTop: 4, opacity: 0.7 }}>
                        {t('subtitle_new_key_hint')}
                    </div>
                )}
            </div>

            {/* Promo Code Input */}
            <div className="glass-card" style={{ padding: 12, display: 'flex', gap: 8, alignItems: 'center' }}>
                <input
                    type="text"
                    value={promoCode}
                    onChange={(e) => {
                        setPromoCode(e.target.value);
                        if (promoStatus !== 'idle') setPromoStatus('idle');
                    }}
                    placeholder={t('promo_placeholder')}
                    style={{
                        flex: 1,
                        background: 'rgba(255,255,255,0.05)',
                        border: '1px solid rgba(255,255,255,0.1)',
                        borderRadius: 8,
                        padding: '10px 12px',
                        color: 'white',
                        fontSize: 14,
                        outline: 'none'
                    }}
                />
                <button
                    onClick={handleApplyPromo}
                    disabled={promoStatus === 'validating' || !promoCode.trim()}
                    className="btn-secondary"
                    style={{
                        padding: '10px 16px',
                        fontSize: 13,
                        opacity: !promoCode.trim() ? 0.5 : 1
                    }}
                >
                    {promoStatus === 'validating' ? '...' : t('promo_apply')}
                </button>
            </div>
            {promoStatus === 'valid' && (
                <div style={{ color: '#34c759', fontSize: 12, marginTop: -8, marginLeft: 4 }}>
                    ✅ {t('promo_applied', { percent: discountPercent })}
                </div>
            )}
            {promoStatus === 'invalid' && (
                <div style={{ color: '#ff3b30', fontSize: 12, marginTop: -8, marginLeft: 4 }}>
                    ❌ {t('promo_invalid')}
                </div>
            )}

            {/* Extend explanation */}
            {isExtend && (
                <div className="tip-box tip-box-info">
                    <span className="tip-icon">ℹ️</span>
                    <span>{t('help_extend_info')}</span>
                </div>
            )}

            {/* New key explanation */}
            {!isExtend && (
                <div className="tip-box tip-box-info">
                    <span className="tip-icon">ℹ️</span>
                    <span>{t('help_new_key_info')}</span>
                </div>
            )}

            {/* Plan cards */}
            <div className="stagger" style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
                {displayPlans.map((plan, idx) => {
                    const originalIdx = plans.findIndex(p => p.label === plan.label && p.days === plan.days);
                    const price = plan.discountedPrice ?? plan.price;
                    const hasDiscount = plan.discountedPrice !== undefined && plan.discountedPrice < plan.price;

                    let checkoutUrl = `/checkout/${originalIdx}?`;
                    if (isExtend) checkoutUrl += `extend=${extendKeyId}&`;
                    if (appliedPromoCode) checkoutUrl += `promo=${encodeURIComponent(appliedPromoCode)}`;

                    return (
                        <Link
                            key={idx}
                            to={checkoutUrl}
                            style={{ textDecoration: 'none', color: 'inherit' }}
                        >
                            <div
                                className={`glass-card ${idx === bestIdx ? 'glass-card-active' : ''}`}
                                style={{
                                    padding: 16,
                                    position: 'relative',
                                    transition: 'transform 0.15s ease',
                                }}
                            >
                                {idx === bestIdx && (
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
                                        <div style={{ fontWeight: 600, fontSize: 15 }}>{plan.label}</div>
                                        <div className="text-hint" style={{ fontSize: 12, marginTop: 3, display: 'flex', alignItems: 'center', gap: 6 }}>
                                            <span>📅 {plan.days} {t('days_left').replace(' left', '')}</span>
                                            <span style={{ opacity: 0.3 }}>·</span>
                                            <span style={plan.traffic_limit_gb === 0 ? { color: '#34c759' } : {}}>
                                                {plan.traffic_limit_gb > 0 ? `📊 ${plan.traffic_limit_gb} GB` : t('unlimited')}
                                            </span>
                                        </div>
                                        {isExtend && (
                                            <div className="text-hint" style={{ fontSize: 10, marginTop: 4, color: '#34c759' }}>
                                                {t('new_expiry', { date: calcNewExpiry(plan.days) })}
                                            </div>
                                        )}
                                    </div>
                                    <div style={{ textAlign: 'right' }}>
                                        <div className="text-link" style={{ fontSize: 18, fontWeight: 700, color: hasDiscount ? '#34c759' : undefined }}>
                                            {price.toLocaleString()}
                                        </div>
                                        {hasDiscount && (
                                            <div style={{ fontSize: 13, textDecoration: 'line-through', opacity: 0.5 }}>
                                                {plan.price.toLocaleString()}
                                            </div>
                                        )}
                                        <div className="text-hint" style={{ fontSize: 11 }}>{plan.currency}</div>
                                        <div className="text-hint" style={{ fontSize: 9, marginTop: 2 }}>
                                            {Math.round(price / plan.days)} {t('per_day', { currency: plan.currency.toLowerCase() })}
                                        </div>
                                    </div>
                                </div>
                            </div>
                        </Link>
                    );
                })}
            </div>

            <div className="tip-box tip-box-success">
                <span className="tip-icon">✅</span>
                <span>{t('help_payments')}</span>
            </div>
        </div>
    );
}
