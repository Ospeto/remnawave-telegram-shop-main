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
    days_remaining: number;
    happ_link: string | null;
}

export function Home() {
    const { initData, tg } = useTelegram();
    const [data, setData] = useState<UserData | null>(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [copied, setCopied] = useState(false);

    useEffect(() => {
        if (tg) {
            tg.BackButton.hide();
        }
    }, [tg]);

    useEffect(() => {
        if (!initData) {
            setLoading(false);
            return;
        }

        fetch('/api/me', {
            headers: { 'Authorization': `twa ${initData}` }
        })
            .then(res => {
                if (!res.ok) throw new Error(`API Error: ${res.status}`);
                return res.json();
            })
            .then(setData)
            .catch(err => setError(err.message))
            .finally(() => setLoading(false));
    }, [initData]);

    const handleCopyLink = () => {
        if (data?.subscription_url) {
            navigator.clipboard.writeText(data.subscription_url).then(() => {
                setCopied(true);
                setTimeout(() => setCopied(false), 2000);
            });
        }
    };

    if (loading) {
        return (
            <div className="flex flex-col items-center justify-center h-screen gap-3">
                <div className="w-8 h-8 border-2 border-blue-500 border-t-transparent rounded-full animate-spin"></div>
                <span className="text-gray-400 text-sm">Loading...</span>
            </div>
        );
    }

    if (!initData) {
        return (
            <div className="flex flex-col items-center justify-center h-screen gap-4 p-6 text-center">
                <div className="text-5xl">📱</div>
                <h1 className="text-xl font-bold">Remnawave Shop</h1>
                <p className="text-gray-500">Please open this app inside Telegram.</p>
            </div>
        );
    }

    if (error) {
        return (
            <div className="p-4 text-center text-red-500">
                <p>Error: {error}</p>
                <button onClick={() => window.location.reload()} className="mt-4 px-4 py-2 bg-gray-200 dark:bg-gray-700 rounded">Retry</button>
            </div>
        );
    }

    const daysText = data?.days_remaining === 1 ? '1 day' : `${data?.days_remaining} days`;

    return (
        <div className="min-h-screen p-4 flex flex-col gap-4">
            {/* Header */}
            <header className="flex items-center gap-3 px-1">
                <div className="w-12 h-12 bg-gradient-to-br from-blue-500 to-purple-600 rounded-full flex items-center justify-center text-white text-lg font-bold shadow-lg">
                    {data?.user?.username?.[0]?.toUpperCase() || 'U'}
                </div>
                <div>
                    <h1 className="text-lg font-bold">{data?.user?.username || 'User'}</h1>
                    <p className="text-xs text-gray-400">VPN Subscription</p>
                </div>
            </header>

            {/* Status Card */}
            <div className={`p-4 rounded-2xl border shadow-sm ${data?.is_active
                    ? 'bg-gradient-to-br from-green-900/30 to-green-800/10 border-green-700/40'
                    : 'bg-gradient-to-br from-gray-800/50 to-gray-700/20 border-gray-600/40'
                }`}>
                <div className="flex items-center justify-between mb-3">
                    <span className="text-xs uppercase tracking-wider text-gray-400">Status</span>
                    <span className={`px-2 py-0.5 rounded-full text-xs font-bold ${data?.is_active
                            ? 'bg-green-500/20 text-green-400'
                            : 'bg-gray-500/20 text-gray-400'
                        }`}>
                        {data?.is_active ? '● Active' : '○ Inactive'}
                    </span>
                </div>

                {data?.is_active ? (
                    <div className="space-y-2">
                        <div className="flex items-baseline gap-2">
                            <span className="text-3xl font-bold text-white">{daysText}</span>
                            <span className="text-sm text-gray-400">remaining</span>
                        </div>
                        {data?.expire_at && (
                            <div className="text-xs text-gray-500">
                                Expires: {new Date(data.expire_at).toLocaleDateString('en-US', {
                                    year: 'numeric', month: 'short', day: 'numeric'
                                })}
                            </div>
                        )}
                        {/* Progress bar */}
                        {data.days_remaining > 0 && (
                            <div className="w-full bg-gray-700/50 rounded-full h-1.5 mt-2">
                                <div
                                    className="bg-green-500 h-1.5 rounded-full transition-all"
                                    style={{ width: `${Math.min(100, (data.days_remaining / 30) * 100)}%` }}
                                ></div>
                            </div>
                        )}
                    </div>
                ) : (
                    <div className="text-gray-400 text-sm">
                        No active subscription. Buy a plan to get started!
                    </div>
                )}
            </div>

            {/* Action Buttons */}
            {data?.is_active && data?.happ_link && (
                <a
                    href={data.happ_link}
                    className="w-full py-3.5 bg-gradient-to-r from-blue-600 to-blue-500 text-white rounded-xl font-bold text-base shadow-lg active:scale-[0.98] transition-transform flex items-center justify-center gap-2 text-center no-underline"
                >
                    🚀 Add to Happ Proxy
                </a>
            )}

            {data?.is_active && data?.subscription_url && (
                <button
                    onClick={handleCopyLink}
                    className={`w-full py-3 rounded-xl font-medium text-sm border transition-all flex items-center justify-center gap-2 ${copied
                            ? 'bg-green-500/10 border-green-500/30 text-green-400'
                            : 'bg-gray-800/50 border-gray-600/30 text-gray-300 active:scale-[0.98]'
                        }`}
                >
                    {copied ? '✅ Copied!' : '📋 Copy Subscription Link'}
                </button>
            )}

            <Link
                to="/plans"
                className="w-full py-3.5 bg-gradient-to-r from-emerald-600 to-green-500 text-white rounded-xl font-bold text-base shadow-lg active:scale-[0.98] transition-transform flex items-center justify-center gap-2 text-center no-underline"
            >
                💎 {data?.is_active ? 'Extend Subscription' : 'Buy Subscription'}
            </Link>

            {/* Footer hint */}
            <p className="text-center text-gray-500 text-xs mt-auto pb-2">
                Powered by Remnawave
            </p>
        </div>
    );
}
