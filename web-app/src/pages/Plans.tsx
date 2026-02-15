import { useEffect, useState } from 'react';
import { useTelegram } from '../lib/twa';
import { Link } from 'react-router-dom';

interface Plan {
    label: string;
    days: number;
    price: number;
    traffic_limit_gb: number;
    currency: string;
}

export function Plans() {
    const { tg } = useTelegram();
    const [plans, setPlans] = useState<Plan[]>([]);
    const [loading, setLoading] = useState(true);

    useEffect(() => {
        if (tg) {
            tg.BackButton.show();
            tg.BackButton.onClick(() => window.history.back());
        }
        return () => {
            if (tg) {
                tg.BackButton.hide();
                tg.BackButton.offClick(() => window.history.back());
            }
        };
    }, [tg]);

    useEffect(() => {
        fetch('/api/plans')
            .then(res => res.json())
            .then(setPlans)
            .finally(() => setLoading(false));
    }, []);

    if (loading) return <div className="text-center p-8">Loading plans...</div>;

    return (
        <div className="min-h-screen p-4 flex flex-col gap-4">
            <h1 className="text-2xl font-bold mb-2">Select a Plan</h1>

            <div className="grid gap-4">
                {plans.map((plan, index) => (
                    <div key={index} className="bg-white dark:bg-gray-800 p-4 rounded-xl shadow-sm border border-gray-100 dark:border-gray-700 relative overflow-hidden">
                        <div className="flex justify-between items-start mb-2">
                            <div>
                                <h3 className="text-lg font-bold">{plan.label}</h3>
                                <div className="text-sm text-gray-500">{plan.days} Days • {plan.traffic_limit_gb} GB</div>
                            </div>
                            <div className="text-xl font-bold text-[#007AFF]">
                                {plan.price.toLocaleString()} {plan.currency}
                            </div>
                        </div>

                        <Link
                            to={`/checkout/${index}`}
                            className="block w-full py-3 bg-gray-100 dark:bg-gray-700 text-center rounded-lg font-medium hover:bg-[#007AFF] hover:text-white transition-colors"
                        >
                            Choose Plan
                        </Link>
                    </div>
                ))}
            </div>
        </div>
    );
}
