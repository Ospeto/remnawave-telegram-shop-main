import { useCallback, useEffect, useState, type CSSProperties } from 'react';
import { Link, useNavigate } from 'react-router-dom';
import { ErrorScreen } from '../components/ErrorScreen';
import { LoadingScreen } from '../components/LoadingScreen';
import { SessionExpiredScreen } from '../components/SessionExpiredScreen';
import { clearTelegramSession, fetchUserScopedJSONWithTelegramAuth } from '../lib/auth';
import { isAPIStatus } from '../lib/http';
import { useLanguage } from '../lib/LanguageContext';
import { UserData } from '../lib/types';
import { useMXBrownSound } from '../lib/useMXBrownSound';
import { useTelegram } from '../lib/twa';

type ToolCard = {
    to: string;
    titleKey: string;
    subtitleKey: string;
    icon: JSX.Element;
};

export function AdminHub() {
    const { tg, initData, close } = useTelegram();
    const { t } = useLanguage();
    const navigate = useNavigate();
    const { playClick } = useMXBrownSound();
    const [loading, setLoading] = useState(true);
    const [authExpired, setAuthExpired] = useState(false);
    const [accessDenied, setAccessDenied] = useState(false);
    const [error, setError] = useState<string | null>(null);

    const handleBack = useCallback(() => {
        navigate('/');
    }, [navigate]);

    const load = useCallback(async () => {
        if (!initData) {
            setLoading(false);
            return;
        }

        setLoading(true);
        setError(null);
        setAuthExpired(false);
        setAccessDenied(false);

        try {
            const meData = await fetchUserScopedJSONWithTelegramAuth<UserData>(
                '/api/me',
                initData,
                tg?.initDataUnsafe?.user?.id,
            );
            if (!meData.is_admin) {
                setAccessDenied(true);
                return;
            }
        } catch (err) {
            if (isAPIStatus(err, 401)) {
                clearTelegramSession();
                setAuthExpired(true);
                return;
            }
            if (isAPIStatus(err, 403)) {
                setAccessDenied(true);
                return;
            }
            setError(err instanceof Error ? err.message : t('admin_hub_forbidden'));
        } finally {
            setLoading(false);
        }
    }, [initData, t, tg]);

    useEffect(() => {
        if (!tg) return;
        tg.BackButton.show();
        tg.BackButton.onClick(handleBack);
        return () => tg.BackButton.offClick(handleBack);
    }, [handleBack, tg]);

    useEffect(() => {
        void load();
    }, [load]);

    if (loading) return <LoadingScreen message={t('admin_hub_loading')} />;
    if (authExpired) {
        return (
            <SessionExpiredScreen
                title={t('session_expired_title')}
                message={t('session_expired_desc')}
                reloadLabel={t('session_expired_reload')}
                closeLabel={t('session_expired_close')}
                onReload={() => { window.location.reload(); }}
                onClose={() => { close(); }}
            />
        );
    }
    if (!initData) {
        return (
            <div className="screen-center">
                <div style={{ fontSize: 48 }}>📱</div>
                <h1 style={{ fontSize: 20, fontWeight: 700, margin: 0 }}>Wavy Private Server Shop</h1>
                <p className="text-hint" style={{ margin: 0 }}>{t('open_in_tg')}</p>
            </div>
        );
    }
    if (accessDenied) {
        return <ErrorScreen message={t('admin_hub_forbidden')} />;
    }
    if (error) {
        return (
            <ErrorScreen
                message={error}
                onRetry={() => { void load(); }}
                retryLabel={t('retry')}
            />
        );
    }

    const tools: ToolCard[] = [
        {
            to: '/admin/finance',
            titleKey: 'admin_finance_card_title',
            subtitleKey: 'admin_finance_card_subtitle',
            icon: (
                <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M4 19V5" />
                    <path d="M4 19h16" />
                    <path d="M8 15l3-4 3 2 4-6" />
                </svg>
            ),
        },
        {
            to: '/admin/plans',
            titleKey: 'admin_plans_card_title',
            subtitleKey: 'admin_plans_card_subtitle',
            icon: (
                <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M4 6h16" />
                    <path d="M4 12h16" />
                    <path d="M4 18h16" />
                    <path d="M8 3v18" />
                </svg>
            ),
        },
        {
            to: '/admin/promos',
            titleKey: 'admin_promos_card_title',
            subtitleKey: 'admin_promos_card_subtitle',
            icon: (
                <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M4 7h16" />
                    <path d="M7 12h10" />
                    <path d="M9 17h6" />
                </svg>
            ),
        },
    ];

    const cardStyle: CSSProperties = {
        padding: '16px 20px',
        display: 'flex',
        alignItems: 'center',
        gap: 14,
        textDecoration: 'none',
        color: 'var(--digital-card-text)',
        transition: 'transform 0.15s cubic-bezier(0.34, 1.56, 0.64, 1)',
        cursor: 'pointer',
    };

    return (
        <div className="animate-fade-in" style={{ padding: '20px 16px 32px', display: 'grid', gap: 16 }}>
            <header style={{ display: 'grid', gap: 4 }}>
                <h1 style={{ margin: 0, fontSize: 'var(--font-h2)', fontWeight: 'var(--weight-bold)' }}>
                    {t('admin_hub_title')}
                </h1>
                <p className="text-hint" style={{ margin: 0, fontSize: 'var(--font-caption)' }}>
                    {t('admin_hub_subtitle')}
                </p>
            </header>

            <section style={{ display: 'grid', gap: 10 }}>
                <h2 style={{ margin: 0, fontSize: 13, fontWeight: 600, color: 'var(--digital-card-hint)', letterSpacing: '0.4px', textTransform: 'uppercase' }}>
                    {t('admin_hub_section_shop')}
                </h2>
                <div style={{ display: 'grid', gridTemplateColumns: '1fr', gap: 10 }}>
                    {tools.map((tool) => (
                        <Link
                            key={tool.to}
                            to={tool.to}
                            className="digital-card animate-slide-up"
                            style={cardStyle}
                            onClick={() => playClick()}
                            onMouseDown={(e) => { e.currentTarget.style.transform = 'scale(0.98)'; }}
                            onMouseUp={(e) => { e.currentTarget.style.transform = 'scale(1)'; }}
                            onTouchStart={(e) => { e.currentTarget.style.transform = 'scale(0.98)'; }}
                            onTouchEnd={(e) => { e.currentTarget.style.transform = 'scale(1)'; }}
                            onMouseLeave={(e) => { e.currentTarget.style.transform = 'scale(1)'; }}
                        >
                            <div style={{
                                width: 44, height: 44, borderRadius: 12,
                                background: 'var(--digital-card-inner-bg)',
                                backdropFilter: 'blur(10px)',
                                display: 'flex', alignItems: 'center', justifyContent: 'center',
                                color: 'var(--digital-card-text)',
                                boxShadow: '0 4px 12px rgba(0,0,0,0.1)',
                            }} aria-hidden="true">
                                {tool.icon}
                            </div>
                            <div style={{ flex: 1 }}>
                                <div style={{ fontWeight: 'var(--weight-bold)', fontSize: '15px', color: 'var(--digital-card-text)', letterSpacing: '0.2px' }}>
                                    {t(tool.titleKey)}
                                </div>
                                <div style={{ fontSize: '13px', color: 'var(--digital-card-hint)', marginTop: 1 }}>
                                    {t(tool.subtitleKey)}
                                </div>
                            </div>
                            <div style={{
                                width: 28, height: 28, borderRadius: 14,
                                background: 'var(--digital-card-inner-bg)',
                                display: 'flex', alignItems: 'center', justifyContent: 'center',
                                fontSize: 14, color: 'var(--digital-card-text)',
                            }} aria-hidden="true">→</div>
                        </Link>
                    ))}
                </div>
            </section>
        </div>
    );
}
