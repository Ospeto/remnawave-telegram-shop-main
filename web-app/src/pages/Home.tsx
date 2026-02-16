import { useEffect, useState } from 'react';
import { useTelegram } from '../lib/twa';
import { Link } from 'react-router-dom';
import { useLanguage } from '../lib/LanguageContext';

interface SubscriptionKey {
    id: number;
    label: string;
    username: string;
    subscription_url: string;
    happ_link: string;
    expire_at: string | null;
    days_remaining: number;
    status: string;
    traffic_used_gb: number;
    traffic_limit_gb: number;
}

interface UserData {
    user: {
        id: number;
        telegram_id: number;
    };
    keys: SubscriptionKey[];
    is_active: boolean;
    expire_at: string | null;
    days_remaining: number;
}

export function Home() {
    const { initData, tg } = useTelegram();
    const { t, language, setLanguage } = useLanguage();
    const [data, setData] = useState<UserData | null>(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [copiedId, setCopiedId] = useState<number | null>(null);

    useEffect(() => {
        if (tg) tg.BackButton.hide();
    }, [tg]);

    useEffect(() => {
        if (!initData) { setLoading(false); return; }
        fetch('/api/me', { headers: { 'Authorization': `twa ${initData}` } })
            .then(res => { if (!res.ok) throw new Error(`${res.status}`); return res.json(); })
            .then(setData)
            .catch(err => {
                setError(`${err.name}: ${err.message}`);
            })
            .finally(() => setLoading(false));
    }, [initData]);

    const handleCopy = (url: string, id: number) => {
        navigator.clipboard.writeText(url).then(() => {
            setCopiedId(id);
            setTimeout(() => setCopiedId(null), 2000);
        });
    };

    const toggleLanguage = () => {
        setLanguage(language === 'en' ? 'my' : 'en');
    };

    if (loading) return (
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: '100vh', gap: 12 }}>
            <div className="spinner" />
            <span className="text-hint" style={{ fontSize: 13 }}>{t('loading')}</span>
        </div>
    );

    if (!initData) return (
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: '100vh', gap: 16, padding: 24, textAlign: 'center' }}>
            <div style={{ fontSize: 48 }}>📱</div>
            <h1 style={{ fontSize: 20, fontWeight: 700, margin: 0 }}>Remnawave Shop</h1>
            <p className="text-hint" style={{ margin: 0 }}>{t('open_in_tg')}</p>
        </div>
    );

    if (error) return (
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: '100vh', gap: 16, padding: 24 }}>
            <div style={{ fontSize: 48 }}>⚠️</div>
            <p style={{ color: '#ff3b30' }}>{t('error_prefix')} {error}</p>
            <button className="btn-secondary" onClick={() => window.location.reload()}>{t('retry')}</button>
        </div>
    );

    const keys = data?.keys || [];
    const activeKeys = keys.filter(k => k.status === 'active');

    return (
        <div className="animate-fade-in" style={{ padding: 16, display: 'flex', flexDirection: 'column', gap: 16, minHeight: '100vh' }}>
            {/* Header */}
            <div style={{ display: 'flex', alignItems: 'center', gap: 12, padding: '4px 0' }}>
                <div style={{
                    width: 44, height: 44,
                    background: 'linear-gradient(135deg, #5ebbff, #007AFF)',
                    borderRadius: '50%',
                    display: 'flex', alignItems: 'center', justifyContent: 'center',
                    color: '#fff', fontWeight: 700, fontSize: 18,
                    boxShadow: '0 4px 16px rgba(94, 187, 255, 0.3)',
                    flexShrink: 0
                }}>
                    🛡️
                </div>
                <div style={{ flex: 1 }}>
                    <h1 style={{ fontSize: 18, fontWeight: 700, margin: 0 }}>{t('home_title')}</h1>
                    <p className="text-hint" style={{ fontSize: 12, margin: 0 }}>
                        {activeKeys.length > 0
                            ? (activeKeys.length === 1 ? t('active_key_count', { count: 1 }) : t('active_key_count_plural', { count: activeKeys.length }))
                            : t('no_active_keys')}
                    </p>
                </div>
                {/* Language Switcher */}
                <button
                    onClick={toggleLanguage}
                    className="btn-secondary"
                    style={{ width: 'auto', padding: '8px 12px', fontSize: 14, borderRadius: 20 }}
                >
                    {language === 'en' ? '🇺🇸 EN' : '🇲🇲 MY'}
                </button>
            </div>

            {/* Key Cards */}
            {keys.length > 0 ? (
                <div className="stagger" style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
                    {keys.map(key => (
                        <div key={key.id} className={`glass-card ${key.status === 'active' ? 'glass-card-success' : ''}`} style={{ padding: 16 }}>
                            {/* Key header */}
                            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 12 }}>
                                <div>
                                    <div style={{ fontWeight: 600, fontSize: 15 }}>{key.label || key.username}</div>
                                    {key.username && key.label && (
                                        <div className="text-hint" style={{ fontSize: 11, marginTop: 2 }}>{key.username}</div>
                                    )}
                                </div>
                                <span className={`badge ${key.status === 'active' ? 'badge-active' : 'badge-expired'}`}>
                                    {key.status === 'active' ? t('key_active') : t('key_expired')}
                                </span>
                            </div>

                            {/* Days remaining */}
                            {key.status === 'active' && (
                                <div style={{ marginBottom: 16 }}>
                                    <div style={{ display: 'flex', alignItems: 'baseline', gap: 6 }}>
                                        <span style={{ fontSize: 28, fontWeight: 700, lineHeight: 1 }}>{key.days_remaining}</span>
                                        <span className="text-hint" style={{ fontSize: 13 }}>{t('days_left')}</span>
                                    </div>
                                    {key.expire_at && (
                                        <div className="text-hint" style={{ fontSize: 11, marginTop: 4 }}>
                                            {t('expires_on', { date: new Date(key.expire_at).toLocaleDateString(language === 'en' ? 'en-US' : 'my-MM', { month: 'short', day: 'numeric', year: 'numeric' }) })}
                                        </div>
                                    )}
                                    {/* Days Progress bar */}
                                    <div style={{ height: 3, background: 'rgba(255,255,255,0.06)', borderRadius: 2, marginTop: 8 }}>
                                        <div style={{
                                            height: '100%', borderRadius: 2,
                                            background: key.days_remaining > 7 ? '#34c759' : key.days_remaining > 3 ? '#ff9f0a' : '#ff3b30',
                                            width: `${Math.min(100, (key.days_remaining / 30) * 100)}%`,
                                            transition: 'width 0.5s ease'
                                        }} />
                                    </div>

                                    {/* Traffic Usage (if limit exists) */}
                                    {key.traffic_limit_gb > 0 && (
                                        <div style={{ marginTop: 12 }}>
                                            <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 11, marginBottom: 4 }}>
                                                <span className="text-hint">{t('data_usage')}</span>
                                                <span style={{ fontWeight: 600 }}>
                                                    {key.traffic_used_gb.toFixed(1)} / {key.traffic_limit_gb.toFixed(0)} GB
                                                </span>
                                            </div>
                                            <div style={{ height: 3, background: 'rgba(255,255,255,0.06)', borderRadius: 2 }}>
                                                <div style={{
                                                    height: '100%', borderRadius: 2,
                                                    background: (key.traffic_used_gb / key.traffic_limit_gb) > 0.9 ? '#ff3b30' : (key.traffic_used_gb / key.traffic_limit_gb) > 0.75 ? '#ff9f0a' : '#007AFF',
                                                    width: `${Math.min(100, (key.traffic_used_gb / key.traffic_limit_gb) * 100)}%`,
                                                    transition: 'width 0.5s ease'
                                                }} />
                                            </div>
                                        </div>
                                    )}
                                </div>
                            )}

                            {/* Expired key help */}
                            {key.status !== 'active' && (
                                <div className="tip-box tip-box-warning" style={{ marginBottom: 12 }}>
                                    <span className="tip-icon">💡</span>
                                    <span>{t('help_expired')}</span>
                                </div>
                            )}

                            {/* Action buttons */}
                            <div style={{ display: 'flex', gap: 8 }}>
                                {key.status === 'active' && key.happ_link && (
                                    <button
                                        className="btn-primary"
                                        style={{ flex: 1, fontSize: 13, padding: '10px 12px' }}
                                        onClick={() => {
                                            const redirectUrl = `${window.location.origin}/redirect?url=${encodeURIComponent(key.happ_link)}`;
                                            if (tg?.openLink) {
                                                tg.openLink(redirectUrl);
                                            } else {
                                                window.open(redirectUrl, '_blank');
                                            }
                                        }}
                                    >
                                        {t('btn_add_happ')}
                                    </button>
                                )}
                                <Link to={`/plans?extend=${key.id}`} className="btn-secondary" style={{ flex: 1, fontSize: 13, padding: '10px 12px', textDecoration: 'none' }}>
                                    {t('btn_extend')}
                                </Link>
                                <button
                                    className="btn-secondary"
                                    onClick={() => handleCopy(key.subscription_url, key.id)}
                                    style={{ flex: 0, minWidth: 44, padding: '10px 12px', fontSize: 13 }}
                                >
                                    {copiedId === key.id ? '✅' : '📋'}
                                </button>
                            </div>

                            {/* Button explanations */}
                            <div className="text-hint" style={{ fontSize: 10, marginTop: 8, lineHeight: 1.6, padding: '0 2px' }}>
                                {key.status === 'active' && key.happ_link && (
                                    <>{t('help_btn_add')} · </>
                                )}
                                {t('help_btn_extend')} · <strong>📋</strong> — {t('help_btn_copy')}
                            </div>
                        </div>
                    ))}
                </div>
            ) : (
                /* Empty state — first-time user welcome */
                <div className="glass-card" style={{ padding: 28, textAlign: 'center' }}>
                    <div style={{ fontSize: 48, marginBottom: 12 }}>👋</div>
                    <h2 style={{ fontSize: 18, fontWeight: 700, margin: '0 0 8px' }}>{t('welcome_title')}</h2>
                    <p className="text-hint" style={{ fontSize: 13, margin: '0 0 16px', lineHeight: 1.6 }}>
                        {t('welcome_text')}
                    </p>

                    {/* Download App Prompt */}
                    <div className="glass-card" style={{ background: 'rgba(0, 122, 255, 0.1)', border: '1px solid rgba(0, 122, 255, 0.2)', padding: 16, marginBottom: 16, textAlign: 'left' }}>
                        <h3 style={{ fontSize: 14, fontWeight: 700, margin: '0 0 8px', display: 'flex', alignItems: 'center', gap: 6 }}>
                            <span style={{ fontSize: 18 }}>📲</span> {t('download_title')}
                        </h3>
                        <p style={{ fontSize: 12, margin: '0 0 12px', opacity: 0.9 }}>
                            {t('download_text')}
                        </p>
                        <div style={{ display: 'flex', gap: 8 }}>
                            <a href="https://play.google.com/store/apps/details?id=com.happ.vpn" target="_blank" className="btn-secondary" style={{ flex: 1, fontSize: 12, padding: '8px', textDecoration: 'none', lineHeight: 1.2, height: 'auto', textAlign: 'center' }}>
                                🤖 Android
                            </a>
                            <a href="https://apps.apple.com/app/id6466666666" target="_blank" className="btn-secondary" style={{ flex: 1, fontSize: 12, padding: '8px', textDecoration: 'none', lineHeight: 1.2, height: 'auto', textAlign: 'center' }}>
                                🍎 iOS
                            </a>
                        </div>
                    </div>

                    <div className="tip-box tip-box-info" style={{ textAlign: 'left', marginBottom: 0 }}>
                        <span className="tip-icon">ℹ️</span>
                        <div>
                            <strong style={{ color: 'var(--tg-text)', fontSize: 12 }}>{t('how_it_works')}</strong>
                            <div style={{ marginTop: 4 }}>
                                <div className="step-row" style={{ padding: '4px 0' }}>
                                    <span className="step-number" style={{ width: 20, height: 20, fontSize: 10 }}>1</span>
                                    <span className="step-text" style={{ fontSize: 11 }}>{t('step_1')}</span>
                                </div>
                                <div className="step-row" style={{ padding: '4px 0' }}>
                                    <span className="step-number" style={{ width: 20, height: 20, fontSize: 10 }}>2</span>
                                    <span className="step-text" style={{ fontSize: 11 }}>{t('step_2')}</span>
                                </div>
                                <div className="step-row" style={{ padding: '4px 0' }}>
                                    <span className="step-number" style={{ width: 20, height: 20, fontSize: 10 }}>3</span>
                                    <span className="step-text" style={{ fontSize: 11 }}>{t('step_3')}</span>
                                </div>
                                <div className="step-row" style={{ padding: '4px 0' }}>
                                    <span className="step-number" style={{ width: 20, height: 20, fontSize: 10 }}>4</span>
                                    <span className="step-text" style={{ fontSize: 11 }}>{t('step_4')}</span>
                                </div>
                            </div>
                        </div>
                    </div>
                </div>
            )}

            {/* Spacer */}
            <div style={{ flex: 1 }} />

            {/* Buy new key button */}
            <Link to="/plans" className="btn-primary animate-slide-up" style={{ textDecoration: 'none' }}>
                {keys.length > 0 ? t('btn_buy_new') : t('btn_get_started')}
            </Link>

            {keys.length > 0 && (
                <div className="tip-box tip-box-info">
                    <span className="tip-icon">💡</span>
                    <span>{t('tip_multi_key')}</span>
                </div>
            )}

            <p className="text-hint" style={{ textAlign: 'center', fontSize: 11, margin: '0 0 8px' }}>
                {t('powered_by')}
            </p>
        </div>
    );
}
