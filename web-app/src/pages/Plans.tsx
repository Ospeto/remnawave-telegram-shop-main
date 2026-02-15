import { useEffect, useState } from 'react';
import { useTelegram } from '../lib/twa';
import { Link, useNavigate, useSearchParams } from 'react-router-dom';

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
    const navigate = useNavigate();
    const [searchParams] = useSearchParams();
    const [plans, setPlans] = useState<Plan[]>([]);
    const [userData, setUserData] = useState<UserData | null>(null);
    const [loading, setLoading] = useState(true);

    const extendKeyId = searchParams.get('extend');
    const isExtend = !!extendKeyId;

    // Find the key being extended
    const extendingKey = userData?.keys?.find(k => k.id === Number(extendKeyId));
    const currentExpiry = extendingKey?.expire_at ? new Date(extendingKey.expire_at) : null;

    useEffect(() => {
        if (tg) {
            tg.BackButton.show();
            tg.BackButton.onClick(() => navigate('/'));
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
            .catch(console.error)
            .finally(() => setLoading(false));
    }, [initData]);

    if (loading) return (
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: '100vh', gap: 12 }}>
            <div className="spinner" />
            <span className="text-hint" style={{ fontSize: 13 }}>Loading plans...</span>
        </div>
    );

    const calcNewExpiry = (days: number) => {
        const base = currentExpiry && currentExpiry > new Date() ? currentExpiry : new Date();
        const d = new Date(base);
        d.setDate(d.getDate() + days);
        return d.toLocaleDateString('en-US', { year: 'numeric', month: 'short', day: 'numeric' });
    };

    // Filter plans by extending key's type: unlimited keys → unlimited plans, limited → limited
    const extendingTrafficLimit = extendingKey ? (extendingKey as any).traffic_limit_gb ?? null : null;
    const filteredPlans = isExtend && extendingTrafficLimit !== null
        ? plans.filter(p =>
            extendingTrafficLimit === 0
                ? p.traffic_limit_gb === 0       // unlimited key → unlimited plans only
                : p.traffic_limit_gb > 0         // limited key → limited plans only
        )
        : plans;

    // Best value = lowest price per day (within filtered set)
    const bestIdx = filteredPlans.length > 0
        ? filteredPlans.reduce((b, p, i) => (p.price / p.days) < (filteredPlans[b].price / filteredPlans[b].days) ? i : b, 0)
        : -1;

    return (
        <div className="animate-fade-in" style={{ padding: 16, display: 'flex', flexDirection: 'column', gap: 16, minHeight: '100vh' }}>
            {/* Header */}
            <div style={{ textAlign: 'center', padding: '8px 0' }}>
                <h1 style={{ fontSize: 20, fontWeight: 700, margin: 0 }}>
                    {isExtend ? '⏳ Extend Key' : '💎 Choose a Plan'}
                </h1>
                {isExtend && extendingKey && (
                    <p className="text-hint" style={{ fontSize: 12, margin: '6px 0 0' }}>
                        Extending: <strong style={{ color: 'var(--tg-text)' }}>{extendingKey.label}</strong>
                        {currentExpiry && <> · Expires {currentExpiry.toLocaleDateString('en-US', { month: 'short', day: 'numeric' })}</>}
                    </p>
                )}
                {!isExtend && (
                    <p className="text-hint" style={{ fontSize: 12, margin: '6px 0 0' }}>
                        A new subscription key will be created
                    </p>
                )}
                {isExtend && filteredPlans.length < plans.length && (
                    <p className="text-hint" style={{ fontSize: 11, margin: '4px 0 0', opacity: 0.7 }}>
                        Only {extendingTrafficLimit === 0 ? 'unlimited' : 'limited'} plans shown for this key type
                    </p>
                )}
            </div>

            {/* Plan cards */}
            <div className="stagger" style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
                {filteredPlans.map((plan, idx) => {
                    const originalIdx = plans.indexOf(plan);
                    return (
                        <Link
                            key={originalIdx}
                            to={`/checkout/${originalIdx}${isExtend ? `?extend=${extendKeyId}` : ''}`}
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
                                        Best Value
                                    </div>
                                )}

                                <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                                    <div>
                                        <div style={{ fontWeight: 600, fontSize: 15 }}>{plan.label}</div>
                                        <div className="text-hint" style={{ fontSize: 12, marginTop: 3, display: 'flex', alignItems: 'center', gap: 6 }}>
                                            <span>{plan.days} days</span>
                                            <span style={{ opacity: 0.3 }}>·</span>
                                            <span style={plan.traffic_limit_gb === 0 ? { color: '#34c759' } : {}}>
                                                {plan.traffic_limit_gb > 0 ? `${plan.traffic_limit_gb} GB` : 'Unlimited'}
                                            </span>
                                        </div>
                                        {isExtend && (
                                            <div className="text-hint" style={{ fontSize: 10, marginTop: 4 }}>
                                                New expiry: {calcNewExpiry(plan.days)}
                                            </div>
                                        )}
                                    </div>
                                    <div style={{ textAlign: 'right' }}>
                                        <div className="text-link" style={{ fontSize: 18, fontWeight: 700 }}>
                                            {plan.price.toLocaleString()}
                                        </div>
                                        <div className="text-hint" style={{ fontSize: 11 }}>{plan.currency}</div>
                                    </div>
                                </div>
                            </div>
                        </Link>
                    );
                })}
            </div>

            <p className="text-hint" style={{ textAlign: 'center', fontSize: 11, margin: '4px 0 0' }}>
                KPay · Wave · AYA Pay
            </p>
        </div>
    );
}
