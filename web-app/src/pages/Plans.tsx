import { useEffect, useState } from 'react';
import { useTelegram } from '../lib/twa';
import { Link, useNavigate } from 'react-router-dom';

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
}

export function Plans() {
    const { tg, initData } = useTelegram();
    const navigate = useNavigate();
    const [plans, setPlans] = useState<Plan[]>([]);
    const [userData, setUserData] = useState<UserData | null>(null);
    const [loading, setLoading] = useState(true);

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
            .then(([plansData, meData]) => {
                setPlans(plansData || []);
                setUserData(meData);
            })
            .catch(console.error)
            .finally(() => setLoading(false));
    }, [initData]);

    if (loading) {
        return (
            <div className="flex flex-col items-center justify-center h-screen gap-3">
                <div className="w-8 h-8 border-2 border-blue-500 border-t-transparent rounded-full animate-spin"></div>
                <span className="text-gray-400 text-sm">Loading plans...</span>
            </div>
        );
    }

    const isExtend = userData?.is_active ?? false;
    const currentExpiry = userData?.expire_at ? new Date(userData.expire_at) : null;

    const calcNewExpiry = (days: number) => {
        const base = currentExpiry && currentExpiry > new Date() ? currentExpiry : new Date();
        const newDate = new Date(base);
        newDate.setDate(newDate.getDate() + days);
        return newDate.toLocaleDateString('en-US', { year: 'numeric', month: 'short', day: 'numeric' });
    };

    // Find best value (lowest price per day)
    const bestValueIdx = plans.length > 0
        ? plans.reduce((best, plan, idx) =>
            (plan.price / plan.days) < (plans[best].price / plans[best].days) ? idx : best, 0)
        : -1;

    return (
        <div className="min-h-screen p-4 flex flex-col gap-4">
            <header className="text-center">
                <h1 className="text-xl font-bold">
                    {isExtend ? '⏳ Extend Subscription' : '💎 Choose a Plan'}
                </h1>
                {isExtend && currentExpiry && (
                    <p className="text-xs text-gray-400 mt-1">
                        Current expiry: {currentExpiry.toLocaleDateString('en-US', { month: 'short', day: 'numeric', year: 'numeric' })}
                    </p>
                )}
            </header>

            <div className="flex flex-col gap-3">
                {plans.map((plan, idx) => (
                    <Link
                        key={idx}
                        to={`/checkout/${idx}`}
                        className="block no-underline"
                    >
                        <div className={`relative p-4 rounded-xl border transition-all active:scale-[0.98] ${idx === bestValueIdx
                            ? 'bg-gradient-to-r from-blue-900/40 to-blue-800/20 border-blue-500/50 shadow-lg shadow-blue-500/10'
                            : 'bg-white dark:bg-gray-800 border-gray-200 dark:border-gray-700'
                            }`}>
                            {idx === bestValueIdx && (
                                <span className="absolute -top-2 right-3 px-2 py-0.5 bg-blue-500 text-white text-[10px] font-bold rounded-full uppercase tracking-wide">
                                    Best Value
                                </span>
                            )}

                            <div className="flex items-center justify-between">
                                <div>
                                    <h3 className="font-bold text-base text-white">{plan.label}</h3>
                                    <div className="flex items-center gap-2 mt-1">
                                        <span className="text-xs text-gray-400">
                                            {plan.days} days
                                        </span>
                                        {plan.traffic_limit_gb > 0 && (
                                            <>
                                                <span className="text-gray-600">•</span>
                                                <span className="text-xs text-gray-400">
                                                    {plan.traffic_limit_gb} GB
                                                </span>
                                            </>
                                        )}
                                        {plan.traffic_limit_gb === 0 && (
                                            <>
                                                <span className="text-gray-600">•</span>
                                                <span className="text-xs text-green-400">Unlimited</span>
                                            </>
                                        )}
                                    </div>
                                    {isExtend && (
                                        <div className="text-[10px] text-gray-500 mt-1">
                                            New expiry: {calcNewExpiry(plan.days)}
                                        </div>
                                    )}
                                </div>
                                <div className="text-right">
                                    <div className="text-lg font-bold text-[#007AFF]">
                                        {plan.price.toLocaleString()}
                                    </div>
                                    <div className="text-xs text-gray-500">{plan.currency}</div>
                                </div>
                            </div>
                        </div>
                    </Link>
                ))}
            </div>

            <p className="text-center text-gray-500 text-xs mt-2">
                Payment via KPay, Wave, AYA Pay
            </p>
        </div>
    );
}
