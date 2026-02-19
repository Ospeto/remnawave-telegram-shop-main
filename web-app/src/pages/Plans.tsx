import { useEffect, useState, useCallback } from 'react';
import { useTelegram } from '../lib/twa';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';
import { useLanguage } from '../lib/LanguageContext';
import { LoadingScreen } from '../components/LoadingScreen';
import { TipBox } from '../components/TipBox';

interface Plan {
    label: string;
    days: number;
    price: number;
    traffic_limit_gb: number;
    currency: string;
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
    const [promoCode, setPromoCode] = useState('');
    const [promoStatus, setPromoStatus] = useState<'idle' | 'validating' | 'valid' | 'invalid'>('idle');
    const [discountPercent, setDiscountPercent] = useState<number>(0);
    const [appliedPromoCode, setAppliedPromoCode] = useState('');

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

    useEffect(() => {
        if (!initData) return;
        const headers = { 'Authorization': `twa ${initData}` };
        Promise.all([
            fetch('/api/plans', { headers }).then(r => r.json()),
            fetch('/api/me', { headers }).then(r => r.json()),
        ])
            .then(([p, m]) => { setPlans(p || []); setUserData(m); })
            .catch(err => console.warn('Plans load error:', err))
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

    if (loading) return <LoadingScreen message={t('loading_plans')} />;

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
                <div className="glass-card" style={{ padding: 12, display: 'flex', gap: 8, alignItems: 'center' }}>
                    <input
                        type="text"
                        value={promoCode}
                        onChange={(e) => {
                            setPromoCode(e.target.value);
                            if (promoStatus !== 'idle') setPromoStatus('idle');
                        }}
                        placeholder={t('promo_placeholder')}
                        aria-label={t('promo_placeholder')}
                        style={{
                            flex: 1,
                            background: 'rgba(255,255,255,0.05)',
                            border: '1px solid rgba(255,255,255,0.1)',
                            borderRadius: 8,
                            padding: '10px 12px',
                            color: 'white',
                            fontSize: 14,
                            outline: 'none',
                            // Replace outline:none with a custom focus ring
                            boxShadow: 'none',
                        }}
                        onFocus={e => (e.target.style.border = '1px solid rgba(94, 187, 255, 0.5)')}
                        onBlur={e => (e.target.style.border = '1px solid rgba(255,255,255,0.1)')}
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
                        {promoStatus === 'validating' ? t('promo_validating') : t('promo_apply')}
                    </button>
                </div>
            )}
            {!isWalletTopup && promoStatus === 'valid' && (
                <div role="status" style={{ color: '#34c759', fontSize: 12, marginTop: -8, marginLeft: 4 }}>
                    {t('promo_valid', { percent: String(discountPercent) })}
                </div>
            )}
            {!isWalletTopup && promoStatus === 'invalid' && (
                <div role="alert" style={{ color: '#ff3b30', fontSize: 12, marginTop: -8, marginLeft: 4 }}>
                    {t('promo_invalid')}
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
            <div className="stagger" style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
                {displayItems.map((item: Plan & { isTopUp?: boolean; discountedPrice?: number }, idx: number) => {
                    const originalIdx = isWalletTopup ? -1 : plans.findIndex(p => p.label === item.label && p.days === item.days);
                    const price = item.discountedPrice || item.price;
                    const hasDiscount = item.discountedPrice !== undefined && item.discountedPrice < item.price;

                    let checkoutUrl = `/checkout/${originalIdx}?`;
                    if (isExtend) checkoutUrl += `extend=${extendKeyId}&`;
                    if (isWalletTopup) {
                        checkoutUrl += `walletTopup=true&amount=${item.price}&`;
                    }
                    if (appliedPromoCode) checkoutUrl += `promo=${encodeURIComponent(appliedPromoCode)}`;

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
                                                <span style={item.traffic_limit_gb === 0 ? { color: '#34c759' } : {}}>
                                                    {item.traffic_limit_gb > 0 ? `📊 ${item.traffic_limit_gb} GB` : t('unlimited')}
                                                </span>
                                            </div>
                                        )}
                                        {isExtend && !isWalletTopup && (
                                            <div className="text-hint" style={{ fontSize: 10, marginTop: 4, color: '#34c759' }}>
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
                                        <div className="text-link" style={{ fontSize: 18, fontWeight: 700, color: hasDiscount ? '#34c759' : undefined }}>
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

            <TipBox variant="success" icon="✅">{t('help_payments')}</TipBox>
        </div>
    );
}
