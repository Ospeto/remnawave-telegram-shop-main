import { useEffect, useState, useCallback } from 'react';
import { useTelegram } from '../lib/twa';
import { Link, useNavigate } from 'react-router-dom';
import { useLanguage } from '../lib/LanguageContext';
import { LoadingScreen } from '../components/LoadingScreen';
import { ErrorScreen } from '../components/ErrorScreen';
import { TipBox } from '../components/TipBox';

interface SubscriptionKey {
    id: number;
    label: string;
    username: string;
    subscription_url: string;
    happ_link: string;
    expire_at: string | null;
    days_remaining: number;
    total_days: number;
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
    trial_eligible: boolean;
    trial_days: number;
}

const fetcher = (url: string, headers: HeadersInit) =>
    fetch(url, { headers }).then(res => {
        if (!res.ok) throw new Error(`${res.status}`);
        return res.json();
    });

export function Home() {
    const { initData, tg } = useTelegram();
    const { t, language, setLanguage } = useLanguage();
    const navigate = useNavigate();
    const [data, setData] = useState<UserData | null>(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [copiedId, setCopiedId] = useState<number | null>(null);
    const [trialLoading, setTrialLoading] = useState(false);
    const [trialError, setTrialError] = useState<string | null>(null);

    const authHeaders = initData ? { 'Authorization': `twa ${initData}` } : undefined;

    const loadData = useCallback(() => {
        if (!initData) { setLoading(false); return; }
        setLoading(true);
        fetcher('/api/me', authHeaders!)
            .then(setData)
            .catch(err => setError(`${err.name}: ${err.message}`))
            .finally(() => setLoading(false));
    }, [initData]);

    useEffect(() => {
        if (tg) tg.BackButton.hide();
    }, [tg]);

    useEffect(() => {
        loadData();
    }, [loadData]);

    const handleCopy = (url: string, id: number) => {
        navigator.clipboard.writeText(url).then(() => {
            setCopiedId(id);
            setTimeout(() => setCopiedId(null), 2000);
        });
    };

    const toggleLanguage = () => {
        setLanguage(language === 'en' ? 'my' : 'en');
    };

    const handleTrialActivation = async () => {
        if (!initData || trialLoading) return;
        setTrialLoading(true);
        setTrialError(null);
        try {
            const res = await fetch('/api/trial', {
                method: 'POST',
                headers: { 'Authorization': `twa ${initData}` },
            });
            if (res.status === 409) {
                setTrialError('Trial already used');
                return;
            }
            if (!res.ok) throw new Error(`${res.status}`);
            // Re-fetch data instead of reloading the page
            loadData();
        } catch {
            setTrialError(t('trial_error'));
        } finally {
            setTrialLoading(false);
        }
    };

    const handleHappLink = (happUrl: string) => {
        const iframe = document.createElement('iframe');
        iframe.style.display = 'none';
        iframe.src = happUrl;
        document.body.appendChild(iframe);
        setTimeout(() => iframe.remove(), 3000);
        const redirectUrl = `${window.location.origin}/redirect.html?url=${encodeURIComponent(happUrl)}`;
        if (tg?.openLink) {
            tg.openLink(redirectUrl);
        } else {
            window.open(redirectUrl, '_blank');
        }
    };

    if (loading) return <LoadingScreen message={t('loading')} />;
    if (!initData) return (
        <div className="screen-center">
            <div style={{ fontSize: 48 }}>📱</div>
            <h1 style={{ fontSize: 20, fontWeight: 700, margin: 0 }}>Wavy Private Server Shop</h1>
            <p className="text-hint" style={{ margin: 0 }}>{t('open_in_tg')}</p>
        </div>
    );
    if (error) return (
        <ErrorScreen
            message={`${t('error_prefix')} ${error}`}
            onRetry={loadData}
            retryLabel={t('retry')}
        />
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
                        boxShadow: '0 4px 16px rgba(0, 210, 190, 0.25)',
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
                    aria-label={language === 'en' ? 'Switch to Myanmar' : 'Switch to English'}
                    style={{ width: 'auto', padding: '8px 12px', fontSize: 14, borderRadius: 20 }}
                >
                    {language === 'en' ? '🇺🇸 EN' : '🇲🇲 MY'}
                </button>
            </div>

            {/* Wallet Button */}
            <div style={{ display: 'grid', gridTemplateColumns: '1fr', gap: 10 }}>
                <Link to="/wallet" className="glass-card" style={{
                    padding: '12px 16px', display: 'flex', alignItems: 'center', gap: 12,
                    textDecoration: 'none', color: 'var(--tg-text)',
                    background: 'rgba(255, 255, 255, 0.05)'
                }}>
                    <div style={{ fontSize: 24 }} aria-hidden="true">👛</div>
                    <div style={{ flex: 1 }}>
                        <div style={{ fontWeight: 600, fontSize: 15 }}>{t('wallet_title')}</div>
                        <div className="text-hint" style={{ fontSize: 12 }}>{t('wallet_subtitle')}</div>
                    </div>
                    <div style={{ fontSize: 18, opacity: 0.5 }} aria-hidden="true">→</div>
                </Link>
            </div>

            {/* Download App Prompt */}
            <div className="glass-card" style={{ background: 'linear-gradient(135deg, rgba(0, 210, 190, 0.06), rgba(0, 180, 220, 0.03))', border: '1px solid rgba(0, 210, 190, 0.15)', padding: 16 }}>
                <h3 style={{ fontSize: 14, fontWeight: 700, margin: '0 0 8px', display: 'flex', alignItems: 'center', gap: 6 }}>
                    {t('download_title')}
                </h3>
                <p style={{ fontSize: 12, margin: '0 0 12px', opacity: 0.9 }}>
                    {t('download_text')}
                </p>
                <div style={{ display: 'flex', gap: 8 }}>
                    <a href="https://play.google.com/store/apps/details?id=com.happproxy&hl=en_US" target="_blank" rel="noopener noreferrer" className="btn-secondary" style={{ flex: 1, fontSize: 12, padding: '8px', textDecoration: 'none', lineHeight: 1.2, height: 'auto', textAlign: 'center' }}>
                        Android
                    </a>
                    <a href="https://apps.apple.com/us/app/happ-proxy-utility/id6504287215" target="_blank" rel="noopener noreferrer" className="btn-secondary" style={{ flex: 1, fontSize: 12, padding: '8px', textDecoration: 'none', lineHeight: 1.2, height: 'auto', textAlign: 'center' }}>
                        iOS
                    </a>
                </div>
            </div>

            {/* Key Cards */}
            {keys.length > 0 ? (
                <div className="stagger" style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
                    {keys.map(key => {
                        // Use total_days if available, otherwise fall back to 30 as a safe default
                        const totalDays = key.total_days > 0 ? key.total_days : 30;
                        const daysPct = Math.min(100, (key.days_remaining / totalDays) * 100);
                        const trafficPct = key.traffic_limit_gb > 0
                            ? Math.min(100, (key.traffic_used_gb / key.traffic_limit_gb) * 100)
                            : 0;

                        return (
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
                                        {/* Days Progress bar — uses total_days from API for correct percentage */}
                                        <div
                                            role="progressbar"
                                            aria-valuemin={0}
                                            aria-valuemax={totalDays}
                                            aria-valuenow={key.days_remaining}
                                            aria-label={`${key.days_remaining} days remaining`}
                                            style={{ height: 4, background: 'rgba(255,255,255,0.08)', borderRadius: 2, marginTop: 12 }}
                                        >
                                            <div style={{
                                                height: '100%', borderRadius: 2,
                                                background: key.days_remaining > 7 ? '#00d2be' : key.days_remaining > 3 ? '#ff9f0a' : '#ff3b30',
                                                width: `${daysPct}%`,
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
                                                <div
                                                    role="progressbar"
                                                    aria-valuemin={0}
                                                    aria-valuemax={key.traffic_limit_gb}
                                                    aria-valuenow={key.traffic_used_gb}
                                                    aria-label={`${key.traffic_used_gb.toFixed(1)} of ${key.traffic_limit_gb} GB used`}
                                                    style={{ height: 4, background: 'rgba(255,255,255,0.08)', borderRadius: 2 }}
                                                >
                                                    <div style={{
                                                        height: '100%', borderRadius: 2,
                                                        background: trafficPct > 90 ? '#ff3b30' : trafficPct > 75 ? '#ff9f0a' : '#00b4dc',
                                                        width: `${trafficPct}%`,
                                                        transition: 'width 0.5s ease'
                                                    }} />
                                                </div>
                                            </div>
                                        )}
                                    </div>
                                )}

                                {/* Expired key help */}
                                {key.status !== 'active' && (
                                    <TipBox variant="warning" icon="💡" style={{ marginBottom: 16 }}>
                                        {t('help_expired')}
                                    </TipBox>
                                )}

                                {/* Action buttons */}
                                <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                                    {key.status === 'active' && key.happ_link && (
                                        <button
                                            className="btn-primary"
                                            style={{ padding: '13px', fontSize: 14, fontWeight: 600 }}
                                            onClick={() => handleHappLink(key.happ_link)}
                                        >
                                            {t('btn_add_happ')}
                                        </button>
                                    )}
                                    <div style={{ display: 'flex', gap: 8 }}>
                                        <Link to={`/plans?extend=${key.id}`} className="btn-secondary" style={{ flex: 1, padding: '12px', fontSize: 14, fontWeight: 600, textDecoration: 'none' }}>
                                            {t('btn_extend')}
                                        </Link>
                                        <button
                                            className="btn-secondary"
                                            onClick={() => handleCopy(key.subscription_url, key.id)}
                                            aria-label={copiedId === key.id ? t('copied') : t('btn_copy_key')}
                                            style={{ flex: 1, padding: '12px', fontSize: 14, fontWeight: 500 }}
                                        >
                                            {copiedId === key.id ? t('copied') : t('btn_copy_key')}
                                        </button>
                                    </div>
                                </div>
                            </div>
                        );
                    })}
                </div>
            ) : (
                /* Empty state — first-time user welcome */
                <div className="glass-card" style={{ padding: 28, textAlign: 'center' }}>
                    <div style={{ fontSize: 48, marginBottom: 12 }} aria-hidden="true">👋</div>
                    <h2 style={{ fontSize: 18, fontWeight: 700, margin: '0 0 8px' }}>{t('welcome_title')}</h2>
                    <p className="text-hint" style={{ fontSize: 13, margin: '0 0 16px', lineHeight: 1.6 }}>
                        {t('welcome_text')}
                    </p>

                    <div className="tip-box tip-box-info" style={{ textAlign: 'left' }}>
                        <span className="tip-icon" aria-hidden="true">ℹ️</span>
                        <div>
                            <strong style={{ color: 'var(--tg-text)', fontSize: 12 }}>{t('how_it_works')}</strong>
                            <div style={{ marginTop: 4 }}>
                                {(['step_1', 'step_2', 'step_3', 'step_4'] as const).map((key, i) => (
                                    <div key={key} className="step-row" style={{ padding: '4px 0' }}>
                                        <span className="step-number" aria-label={`Step ${i + 1}`} style={{ width: 20, height: 20, fontSize: 10 }}>{i + 1}</span>
                                        <span className="step-text" style={{ fontSize: 11 }}>{t(key)}</span>
                                    </div>
                                ))}
                            </div>
                        </div>
                    </div>
                </div>
            )}

            <div style={{ flex: 1 }} />

            {/* Trial Button */}
            {data?.trial_eligible && keys.length === 0 && (
                <>
                    <button
                        className="btn-primary animate-slide-up"
                        onClick={handleTrialActivation}
                        disabled={trialLoading}
                        style={{
                            background: 'linear-gradient(135deg, #00d2be, #00b4dc)',
                            border: 'none',
                            fontSize: 15,
                            fontWeight: 700,
                            letterSpacing: '-0.3px',
                            opacity: trialLoading ? 0.7 : 1,
                        }}
                    >
                        {trialLoading
                            ? t('trial_activating')
                            : t('trial_button', { days: String(data.trial_days) })}
                    </button>
                    {trialError && (
                        <div role="alert" style={{ color: '#ff3b30', fontSize: 12, textAlign: 'center', marginTop: -8 }}>
                            {trialError}
                        </div>
                    )}
                </>
            )}

            {/* Buy new key button */}
            <Link to="/plans" className="btn-primary animate-slide-up" style={{
                textDecoration: 'none',
                ...(data?.trial_eligible && keys.length === 0 ? {
                    background: 'transparent',
                    border: '1px solid rgba(0, 210, 190, 0.3)',
                    color: '#00d2be',
                } : {})
            }}>
                {keys.length > 0 ? t('btn_buy_new') : t('btn_get_started')}
            </Link>

            {keys.length > 0 && (
                <TipBox variant="info" icon="💡">{t('tip_multi_key')}</TipBox>
            )}

            {/* Info Cards */}
            <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 10 }}>
                <div className="glass-card" style={{
                    padding: '16px 12px', textAlign: 'center',
                    background: 'linear-gradient(135deg, rgba(0, 210, 190, 0.06), rgba(0, 180, 220, 0.06))',
                    border: '1px solid rgba(0, 210, 190, 0.15)',
                    display: 'flex', flexDirection: 'column', justifyContent: 'center', alignItems: 'center'
                }}>
                    <div style={{ fontSize: 26, fontWeight: 800, color: '#00d2be', lineHeight: 1, letterSpacing: '-0.5px' }}>
                        {t('info_device_count')}
                    </div>
                    <div className="text-hint" style={{ fontSize: 11, marginTop: 6, letterSpacing: '0.3px', lineHeight: 1.2 }}>
                        {t('info_device_limit')}
                    </div>
                </div>
                <div className="glass-card" style={{
                    padding: '16px 12px', textAlign: 'center',
                    background: 'linear-gradient(135deg, rgba(0, 180, 220, 0.06), rgba(0, 150, 255, 0.06))',
                    border: '1px solid rgba(0, 180, 220, 0.15)',
                    display: 'flex', flexDirection: 'column', justifyContent: 'center', alignItems: 'center'
                }}>
                    <div style={{ fontSize: 26, fontWeight: 800, color: '#00b4dc', lineHeight: 1, letterSpacing: '-0.5px' }}>
                        5
                    </div>
                    <div className="text-hint" style={{ fontSize: 11, marginTop: 6, letterSpacing: '0.3px' }}>
                        {t('info_servers')}
                    </div>
                </div>
            </div>

            <div style={{ textAlign: 'center', fontSize: 11, color: 'rgba(0, 210, 190, 0.45)', letterSpacing: '0.8px', margin: '-2px 0 0', fontWeight: 500 }}>
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
                <svg width="16" height="16" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                    <path d="M11.944 0A12 12 0 0 0 0 12a12 12 0 0 0 12 12 12 12 0 0 0 12-12A12 12 0 0 0 12 0a12 12 0 0 0-.056 0zm4.962 7.224c.1-.002.321.023.465.14a.506.506 0 0 1 .171.325c.016.093.036.306.02.472-.18 1.898-.962 6.502-1.36 8.627-.168.9-.499 1.201-.82 1.23-.696.065-1.225-.46-1.9-.902-1.056-.693-1.653-1.124-2.678-1.8-1.185-.78-.417-1.21.258-1.91.177-.184 3.247-2.977 3.307-3.23.007-.032.014-.15-.056-.212s-.174-.041-.249-.024c-.106.024-1.793 1.14-5.061 3.345-.48.33-.913.49-1.302.48-.428-.008-1.252-.241-1.865-.44-.752-.245-1.349-.374-1.297-.789.027-.216.325-.437.893-.663 3.498-1.524 5.83-2.529 6.998-3.014 3.332-1.386 4.025-1.627 4.476-1.635z" />
                </svg>
                {t('contact_support')}
            </a>
        </div>
    );
}
