import { useEffect, useState } from 'react';
import { useTelegram } from '../lib/twa';
import { Link } from 'react-router-dom';

interface UserData {
    user: {
        id: number;
        telegram_id: number;
        username: string;
    };
    subscription_url: string | null;
    is_active: boolean;
    expire_at: string | null;
}

export function Home() {
    const { initData, openLink } = useTelegram();
    const [data, setData] = useState<UserData | null>(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);

    useEffect(() => {
        if (!initData) {
            setLoading(false);
            return;
        }

        fetch('/api/me', {
            headers: {
                'Authorization': `twa ${initData}`
            }
        })
            .then(res => {
                if (!res.ok) throw new Error(`API Error: ${res.status} ${res.statusText}`);
                return res.json();
            })
            .then(setData)
            .catch(err => {
                console.error('[Home] /api/me error:', err);
                setError(err.message);
            })
            .finally(() => setLoading(false));
    }, [initData]);

    if (loading) {
        return <div className="flex items-center justify-center h-screen animate-pulse">Loading...</div>;
    }

    if (!initData) {
        return (
            <div className="flex flex-col items-center justify-center h-screen gap-4 p-6 text-center">
                <div className="text-5xl">📱</div>
                <h1 className="text-xl font-bold">Remnawave Shop</h1>
                <p className="text-gray-500">Please open this app inside Telegram using the Menu Button.</p>
            </div>
        );
    }

    if (error) {
        return (
            <div className="p-4 text-center text-red-500">
                <p>Error: {error}</p>
                <button onClick={() => window.location.reload()} className="mt-4 px-4 py-2 bg-gray-200 rounded">Retry</button>
            </div>
        );
    }

    return (
        <div className="min-h-screen p-4 flex flex-col gap-4">
            <header className="flex flex-col items-center gap-2">
                <div className="w-16 h-16 bg-blue-500 rounded-full flex items-center justify-center text-white text-2xl font-bold">
                    {data?.user?.username?.[0]?.toUpperCase() || 'U'}
                </div>
                <h1 className="text-xl font-bold">
                    {data?.user?.username || 'User'}
                </h1>
            </header>

            <div className="bg-white dark:bg-gray-800 p-4 rounded-xl shadow-sm border border-gray-100 dark:border-gray-700">
                <h2 className="text-sm uppercase tracking-wider text-gray-500 mb-2">Subscription Status</h2>
                {data?.is_active ? (
                    <div>
                        <div className="text-green-500 font-bold text-lg flex items-center gap-2">
                            <span>●</span> Active
                        </div>
                        <div className="text-sm text-gray-500 mt-1">
                            Expires: {new Date(data?.expire_at!).toLocaleDateString()}
                        </div>
                    </div>
                ) : (
                    <div className="text-gray-500">No active subscription</div>
                )}
            </div>

            {/* Import button for active users with subscription URL */}
            {data?.is_active && data?.subscription_url && (
                <button
                    onClick={() => openLink(data.subscription_url!)}
                    className="w-full py-4 bg-[#007AFF] text-white rounded-xl font-bold text-lg shadow-lg active:scale-95 transition-transform flex items-center justify-center gap-2"
                >
                    <span>🚀</span> Import to Happ Proxy
                </button>
            )}

            {/* Always show Browse Plans button */}
            <Link
                to="/plans"
                className="w-full py-4 bg-[#34C759] text-white rounded-xl font-bold text-lg shadow-lg active:scale-95 transition-transform flex items-center justify-center gap-2 text-center"
            >
                <span>💎</span> {data?.is_active ? 'Extend Subscription' : 'Buy Subscription'}
            </Link>

            <div className="text-center text-gray-400 text-xs mt-2">
                Tap above to view available plans
            </div>
        </div>
    );
}
