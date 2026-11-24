import { useLanguage } from '../lib/LanguageContext';

/**
 * Route-level loading placeholder shown while a lazy page module is fetched.
 * It mirrors the shape of a page (title bar, card, list rows) so the first
 * paint is not frozen on an empty shell.
 */
export function RouteLoadingSkeleton() {
    const { t } = useLanguage();

    return (
        <div className="page-wrapper" role="status" aria-live="polite" aria-busy="true" aria-label={t('loading')}>
            <div style={{ padding: '8px 0', display: 'flex', flexDirection: 'column', gap: 10 }}>
                <div className="skeleton-block" style={{ width: '45%', height: 22, borderRadius: 8 }} />
                <div className="skeleton-block" style={{ width: '70%', height: 12, borderRadius: 6 }} />
            </div>
            <div className="glass-card" style={{ padding: 20, display: 'flex', flexDirection: 'column', gap: 12 }}>
                <div className="skeleton-block" style={{ width: '35%', height: 14, borderRadius: 6 }} />
                <div className="skeleton-block" style={{ width: '100%', height: 48, borderRadius: 12 }} />
                <div className="skeleton-block" style={{ width: '100%', height: 48, borderRadius: 12 }} />
            </div>
            <div className="glass-card" style={{ padding: 20, display: 'flex', flexDirection: 'column', gap: 12 }}>
                <div className="skeleton-block" style={{ width: '55%', height: 14, borderRadius: 6 }} />
                <div className="skeleton-block" style={{ width: '100%', height: 14, borderRadius: 6 }} />
                <div className="skeleton-block" style={{ width: '80%', height: 14, borderRadius: 6 }} />
            </div>
        </div>
    );
}
