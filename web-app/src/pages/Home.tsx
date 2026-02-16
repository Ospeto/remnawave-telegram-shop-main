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
            <h1 style={{ fontSize: 20, fontWeight: 700, margin: 0 }}>Wavy VPN Shop</h1>
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
                <img
                    src="/logo.jpg"
                    alt="Wavy"
                    style={{
                        width: 44, height: 44,
                        borderRadius: '50%',
                        objectFit: 'cover',
                        boxShadow: '0 4px 16px rgba(94, 187, 255, 0.3)',
                        flexShrink: 0
                    }}
                />
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

            {/* Download App Prompt - Visible for all users */}
            <div className="glass-card" style={{ background: 'rgba(0, 122, 255, 0.1)', border: '1px solid rgba(0, 122, 255, 0.2)', padding: 16, textAlign: 'left' }}>
                <h3 style={{ fontSize: 14, fontWeight: 700, margin: '0 0 8px', display: 'flex', alignItems: 'center', gap: 6 }}>
                    {t('download_title')}
                </h3>
                <p style={{ fontSize: 12, margin: '0 0 12px', opacity: 0.9 }}>
                    {t('download_text')}
                </p>
                <div style={{ display: 'flex', gap: 8 }}>
                    <a href="https://play.google.com/store/apps/details?id=com.happproxy&hl=en_US" target="_blank" className="btn-secondary" style={{ flex: 1, fontSize: 12, padding: '8px', textDecoration: 'none', lineHeight: 1.2, height: 'auto', textAlign: 'center' }}>
                        Android
                    </a>
                    <a href="https://apps.apple.com/us/app/happ-proxy-utility/id6504287215" target="_blank" className="btn-secondary" style={{ flex: 1, fontSize: 12, padding: '8px', textDecoration: 'none', lineHeight: 1.2, height: 'auto', textAlign: 'center' }}>
                        iOS
                    </a>
                </div>
            </div>

            {/* Key Cards */}
            {keys.length > 0 ? (
                <div className="stagger" style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
                    {keys.map(key => (
                        <div key={key.id} className={`glass-card ${key.status === 'active' ? 'glass-card-success' : ''}`} style={{ padding: 16 }}>
                            {/* Key header */}
                            <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center', marginBottom: 16 }}>
                                <div>
                                    <div style={{ fontWeight: 700, fontSize: 16, letterSpacing: '-0.3px' }}>{key.label || key.username}</div>
                                    {key.username && key.label && (
                                        <div className="text-hint" style={{ fontSize: 12, marginTop: 2 }}>{key.username}</div>
                                    )}
                                </div>
                                <span className={`badge ${key.status === 'active' ? 'badge-active' : 'badge-expired'}`}>
                                    {key.status === 'active' ? t('key_active') : t('key_expired')}
                                </span>
                            </div>

                            {/* Days remaining */}
                            {key.status === 'active' && (
                                <div style={{ marginBottom: 20 }}>
                                    <div style={{ display: 'flex', alignItems: 'baseline', gap: 6 }}>
                                        <span style={{ fontSize: 32, fontWeight: 800, lineHeight: 1, letterSpacing: '-1px' }}>{key.days_remaining}</span>
                                        <span className="text-hint" style={{ fontSize: 14 }}>{t('days_left')}</span>
                                    </div>
                                    {key.expire_at && (
                                        <div className="text-hint" style={{ fontSize: 12, marginTop: 4, opacity: 0.7 }}>
                                            {t('expires_on', { date: new Date(key.expire_at).toLocaleDateString(language === 'en' ? 'en-US' : 'my-MM', { month: 'short', day: 'numeric', year: 'numeric' }) })}
                                        </div>
                                    )}
                                    {/* Days Progress bar */}
                                    <div style={{ height: 4, background: 'rgba(255,255,255,0.08)', borderRadius: 2, marginTop: 12 }}>
                                        <div style={{
                                            height: '100%', borderRadius: 2,
                                            background: key.days_remaining > 7 ? '#34c759' : key.days_remaining > 3 ? '#ff9f0a' : '#ff3b30',
                                            width: `${Math.min(100, (key.days_remaining / 30) * 100)}%`,
                                            transition: 'width 0.5s ease'
                                        }} />
                                    </div>

                                    {/* Traffic Usage (if limit exists) */}
                                    {key.traffic_limit_gb > 0 && (
                                        <div style={{ marginTop: 14 }}>
                                            <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 12, marginBottom: 6 }}>
                                                <span className="text-hint">{t('data_usage')}</span>
                                                <span style={{ fontWeight: 600, fontFamily: 'monospace' }}>
                                                    {key.traffic_used_gb.toFixed(1)} <span style={{ opacity: 0.5 }}>/</span> {key.traffic_limit_gb.toFixed(0)} GB
                                                </span>
                                            </div>
                                            <div style={{ height: 4, background: 'rgba(255,255,255,0.08)', borderRadius: 2 }}>
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
                                <div className="tip-box tip-box-warning" style={{ marginBottom: 16 }}>
                                    <span className="tip-icon">💡</span>
                                    <span>{t('help_expired')}</span>
                                </div>
                            )}

                            {/* Action buttons */}
                            <div style={{ display: 'flex', gap: 10 }}>
                                {key.status === 'active' && key.happ_link && (
                                    <button
                                        className="btn-primary"
                                        style={{ flex: 2, padding: '12px', fontSize: 14, fontWeight: 600, boxShadow: '0 4px 12px rgba(0,122,255,0.2)' }}
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
                                <Link to={`/plans?extend=${key.id}`} className="btn-secondary" style={{ flex: 1, padding: '12px', fontSize: 14, fontWeight: 600, textDecoration: 'none', justifyContent: 'center' }}>
                                    {t('btn_extend')}
                                </Link>
                            </div>

                            {/* Copy Key button */}
                            <button
                                className="btn-secondary"
                                onClick={() => handleCopy(key.subscription_url, key.id)}
                                style={{
                                    width: '100%', marginTop: 8, padding: '10px',
                                    fontSize: 13, fontWeight: 500, opacity: 0.8,
                                    display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 6
                                }}
                            >
                                {copiedId === key.id ? t('copied') : t('btn_copy_key')}
                            </button>
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

            {/* Info Cards */}
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10 }}>
                {/* Device Limit */}
                <div className="glass-card" style={{
                    padding: '16px 12px', textAlign: 'center',
                    background: 'linear-gradient(135deg, rgba(0, 210, 190, 0.06), rgba(0, 180, 220, 0.06))',
                    border: '1px solid rgba(0, 210, 190, 0.15)'
                }}>
                    <div style={{ fontSize: 26, fontWeight: 800, color: '#00d2be', lineHeight: 1, letterSpacing: '-0.5px' }}>
                        {t('info_device_count')}
                    </div>
                    <div className="text-hint" style={{ fontSize: 11, marginTop: 6, letterSpacing: '0.3px' }}>
                        {t('info_device_limit')}
                    </div>
                </div>
                {/* Server Count */}
                <div className="glass-card" style={{
                    padding: '16px 12px', textAlign: 'center',
                    background: 'linear-gradient(135deg, rgba(0, 180, 220, 0.06), rgba(0, 150, 255, 0.06))',
                    border: '1px solid rgba(0, 180, 220, 0.15)'
                }}>
                    <div style={{ fontSize: 26, fontWeight: 800, color: '#00b4dc', lineHeight: 1, letterSpacing: '-0.5px' }}>
                        5
                    </div>
                    <div className="text-hint" style={{ fontSize: 11, marginTop: 6, letterSpacing: '0.3px' }}>
                        {t('info_servers')}
                    </div>
                </div>
            </div>

            {/* Server locations list */}
            <div style={{
                textAlign: 'center', fontSize: 11,
                color: 'rgba(0, 210, 190, 0.45)',
                letterSpacing: '0.8px', margin: '-2px 0 0',
                fontWeight: 500
            }}>
                {t('info_server_list')}
            </div>

            {/* Contact Support */}
            <a
                href="https://t.me/ospeto"
                target="_blank"
                rel="noopener noreferrer"
                style={{
                    display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 8,
                    padding: '10px 24px', borderRadius: 12,
                    background: 'rgba(0, 136, 204, 0.08)',
                    border: '1px solid rgba(0, 136, 204, 0.15)',
                    color: '#0088cc', fontSize: 13, fontWeight: 600,
                    textDecoration: 'none', transition: 'all 0.2s ease',
                    marginBottom: 8, alignSelf: 'center'
                }}
            >
                <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor">
                    <path d="M11.944 0A12 12 0 0 0 0 12a12 12 0 0 0 12 12 12 12 0 0 0 12-12A12 12 0 0 0 12 0a12 12 0 0 0-.056 0zm4.962 7.224c.1-.002.321.023.465.14a.506.506 0 0 1 .171.325c.016.093.036.306.02.472-.18 1.898-.962 6.502-1.36 8.627-.168.9-.499 1.201-.82 1.23-.696.065-1.225-.46-1.9-.902-1.056-.693-1.653-1.124-2.678-1.8-1.185-.78-.417-1.21.258-1.91.177-.184 3.247-2.977 3.307-3.23.007-.032.014-.15-.056-.212s-.174-.041-.249-.024c-.106.024-1.793 1.14-5.061 3.345-.48.33-.913.49-1.302.48-.428-.008-1.252-.241-1.865-.44-.752-.245-1.349-.374-1.297-.789.027-.216.325-.437.893-.663 3.498-1.524 5.83-2.529 6.998-3.014 3.332-1.386 4.025-1.627 4.476-1.635z" />
                </svg>
                {t('contact_support')}
            </a>
        </div>
    );
}
