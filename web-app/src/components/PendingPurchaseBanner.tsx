import { Link } from 'react-router-dom';
import { useLanguage } from '../lib/LanguageContext';
import { PendingPurchase } from '../lib/types';

interface PendingPurchaseBannerProps {
    pending: PendingPurchase | null;
    /** Where the customer goes to finish or check the order. */
    to?: string;
    onNavigate?: () => void;
}

/**
 * The unfinished-order banner shown on Home and Wallet.
 *
 * U3: a screenshot payment that was never completed stayed invisible until the
 * customer happened to start another purchase and hit the 409 in Checkout. The
 * banner turns that dead order into one tap, and it distinguishes "upload the
 * screenshot" from "we are confirming this right now" so a customer does not
 * resend money for an order the shop is already processing (A9).
 */
export function PendingPurchaseBanner({ pending, to = '/plans', onNavigate }: PendingPurchaseBannerProps) {
    const { t } = useLanguage();
    if (!pending) return null;

    const processing = pending.status === 'processing';
    const amount = (pending.amount || 0).toLocaleString();
    const currency = pending.currency || 'MMK';
    const bodyKey = processing ? 'pending_purchase_banner_processing' : 'pending_purchase_banner_body';
    const actionKey = processing ? 'pending_purchase_banner_processing_action' : 'pending_purchase_banner_action';

    return (
        <div
            role="status"
            aria-live="polite"
            data-testid="pending-purchase-banner"
            style={{
                borderRadius: 16,
                padding: '14px 16px',
                background: 'rgba(255, 159, 10, 0.12)',
                border: '1px solid rgba(255, 159, 10, 0.28)',
                display: 'flex',
                flexDirection: 'column',
                gap: 8,
            }}
        >
            <div style={{ fontWeight: 700, fontSize: 14, color: 'var(--text-color)' }}>
                {t('pending_purchase_banner_title')}
            </div>
            <div className="text-hint" style={{ fontSize: 13, lineHeight: 1.45 }}>
                {t(bodyKey, { amount, currency })}
            </div>
            <Link
                to={to}
                className="btn-secondary"
                onClick={onNavigate}
                style={{ textAlign: 'center', textDecoration: 'none', fontSize: 14, padding: '10px 12px' }}
            >
                {t(actionKey)}
            </Link>
        </div>
    );
}
