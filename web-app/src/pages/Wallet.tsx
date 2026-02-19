import { useEffect, useState, useCallback } from 'react';
import { useTelegram } from '../lib/twa';
import { useNavigate } from 'react-router-dom';
import { useLanguage } from '../lib/LanguageContext';
import { LoadingScreen } from '../components/LoadingScreen';
import { ErrorScreen } from '../components/ErrorScreen';

interface WalletData {
  balance: number;
  currency: string;
}

interface Transaction {
  id: number;
  amount: number;
  type: 'topup' | 'purchase' | 'refund';
  description: string;
  created_at: string;
}

export function Wallet() {
  const { tg, initData } = useTelegram();
  const { t } = useLanguage();
  const navigate = useNavigate();

  const [wallet, setWallet] = useState<WalletData | null>(null);
  const [transactions, setTransactions] = useState<Transaction[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState(false);

  const handleBack = useCallback(() => navigate('/'), [navigate]);

  useEffect(() => {
    if (!tg) return;
    tg.BackButton.show();
    tg.BackButton.onClick(handleBack);
    return () => tg.BackButton.offClick(handleBack);
  }, [tg, handleBack]);

  useEffect(() => {
    if (!initData) return;
    const headers = { 'Authorization': `twa ${initData}` };

    Promise.all([
      fetch('/api/wallet', { headers }).then(r => r.json()),
      fetch('/api/wallet/history?limit=10', { headers }).then(r => r.json()),
    ])
      .then(([walletData, historyData]) => {
        setWallet(walletData);
        setTransactions(historyData || []);
      })
      .catch(err => {
        console.warn('Wallet load error:', err);
        setLoadError(true);
      })
      .finally(() => setLoading(false));
  }, [initData]);

  const getTransactionIcon = (type: string) => {
    switch (type) {
      case 'topup': return '💰';
      case 'purchase': return '🛒';
      case 'refund': return '↩️';
      default: return '📝';
    }
  };

  const getTransactionColor = (type: string) => {
    switch (type) {
      case 'topup': return '#34c759';
      case 'purchase': return '#ff3b30';
      case 'refund': return '#007aff';
      default: return '#999';
    }
  };

  if (loading) return <LoadingScreen message={t('loading_wallet')} />;

  if (loadError || !wallet) {
    return (
      <ErrorScreen
        message={t('wallet_error')}
        onRetry={() => navigate('/')}
        retryLabel={t('go_home')}
      />
    );
  }

  return (
    <div className="animate-fade-in" style={{
      padding: 'var(--layout-padding)',
      display: 'flex',
      flexDirection: 'column',
      gap: 'var(--layout-gap)',
      minHeight: '100vh'
    }}>
      {/* Header */}
      <div style={{ textAlign: 'center', padding: '8px 0' }}>
        <h1 style={{ fontSize: 'var(--font-h1)', fontWeight: 'var(--weight-bold)', margin: 0 }}>{t('wallet_title')}</h1>
        <p className="text-hint" style={{ fontSize: 'var(--font-caption)', margin: '6px 0 0' }}>
          {t('wallet_subtitle')}
        </p>
      </div>

      {/* Balance Card - Premium */}
      <div style={{
        position: 'relative',
        borderRadius: 20,
        padding: '28px 24px 24px',
        background: 'linear-gradient(135deg, #1a2a3a 0%, #0f1f2e 40%, #1a2e22 100%)',
        boxShadow: '0 8px 32px rgba(0,0,0,0.45), 0 0 0 1px rgba(255,255,255,0.07) inset',
        overflow: 'hidden',
      }} className="animate-slide-up">

        {/* Shimmer overlay */}
        <div style={{
          position: 'absolute', inset: 0,
          background: 'linear-gradient(120deg, transparent 30%, rgba(255,255,255,0.04) 50%, transparent 70%)',
          pointerEvents: 'none',
        }} />

        {/* Top row: chip + contactless */}
        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'flex-start', marginBottom: 28 }}>
          {/* EMV Chip */}
          <div style={{
            width: 42, height: 32, borderRadius: 6,
            background: 'linear-gradient(135deg, #c9a84c 0%, #f5d07a 40%, #b8902a 100%)',
            boxShadow: '0 2px 6px rgba(0,0,0,0.4)',
            position: 'relative', overflow: 'hidden',
          }} aria-hidden="true">
            <div style={{ position: 'absolute', top: '50%', left: 0, right: 0, height: 1, background: 'rgba(0,0,0,0.25)', transform: 'translateY(-50%)' }} />
            <div style={{ position: 'absolute', left: '50%', top: 0, bottom: 0, width: 1, background: 'rgba(0,0,0,0.25)', transform: 'translateX(-50%)' }} />
          </div>

          {/* Contactless icon */}
          <svg width="26" height="26" viewBox="0 0 24 24" fill="none" stroke="rgba(255,255,255,0.55)" strokeWidth="1.8" aria-hidden="true">
            <path d="M5 12.55a11 11 0 0 1 14.08 0" />
            <path d="M1.42 9a16 16 0 0 1 21.16 0" />
            <path d="M8.53 16.11a6 6 0 0 1 6.95 0" />
            <line x1="12" y1="20" x2="12.01" y2="20" />
          </svg>
        </div>

        {/* Balance label */}
        <div style={{ fontSize: 12, fontWeight: 500, letterSpacing: '1.2px', textTransform: 'uppercase', color: 'rgba(255,255,255,0.45)', marginBottom: 6 }}>
          {t('current_balance')}
        </div>

        {/* Balance amount */}
        <div style={{
          fontSize: 38, fontWeight: 800, letterSpacing: '-1px',
          color: '#fff',
          textShadow: '0 2px 12px rgba(0,0,0,0.35)',
          marginBottom: 28,
          lineHeight: 1,
        }}>
          {(wallet?.balance || 0).toLocaleString()}
          <span style={{ fontSize: 18, fontWeight: 600, marginLeft: 8, opacity: 0.65 }}>{wallet?.currency || ''}</span>
        </div>

        {/* Divider */}
        <div style={{ height: 1, background: 'rgba(255,255,255,0.08)', marginBottom: 20 }} />

        {/* Top-up button */}
        <button
          onClick={() => navigate('/plans?walletTopup=true')}
          style={{
            width: '100%', padding: '12px',
            borderRadius: 12,
            background: 'rgba(255,255,255,0.1)',
            backdropFilter: 'blur(8px)',
            border: '1px solid rgba(255,255,255,0.15)',
            color: '#fff', fontWeight: 700, fontSize: 15,
            cursor: 'pointer', letterSpacing: '0.2px',
            transition: 'background 0.2s',
          }}
          onMouseOver={e => (e.currentTarget.style.background = 'rgba(255,255,255,0.16)')}
          onMouseOut={e => (e.currentTarget.style.background = 'rgba(255,255,255,0.10)')}
        >
          {t('top_up_wallet')}
        </button>

        {/* Card brand label bottom-right */}
        <div style={{
          position: 'absolute', bottom: 22, right: 24,
          fontSize: 12, fontWeight: 800, letterSpacing: '2px',
          color: 'rgba(255,255,255,0.2)', textTransform: 'uppercase',
        }}>WAVY</div>
      </div>


      {/* Transaction History */}
      <div>
        <h2 style={{ fontSize: 'var(--font-h2)', fontWeight: 'var(--weight-bold)', margin: '16px 0 12px' }}>
          {t('transaction_history')}
        </h2>

        {transactions.length === 0 ? (
          <div className="glass-card" style={{ padding: '32px 24px', textAlign: 'center', background: 'var(--card-bg)' }}>
            <div style={{
              fontSize: 40, marginBottom: 16,
              background: 'var(--btn-sec-bg)', width: 80, height: 80, borderRadius: 40,
              display: 'flex', alignItems: 'center', justifyContent: 'center', margin: '0 auto 16px'
            }} aria-hidden="true">
              📜
            </div>
            <div style={{ fontWeight: 700, fontSize: 17, marginBottom: 6 }}>{t('wallet_empty_title')}</div>
            <div className="text-hint" style={{ fontSize: 13, maxWidth: 280, margin: '0 auto', lineHeight: 1.5 }}>
              {t('wallet_empty_desc')}
            </div>
          </div>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            {transactions.map((tx) => (
              <div key={tx.id} className="glass-card" style={{ padding: 14, display: 'flex', alignItems: 'center', gap: 12 }}>
                <div style={{ fontSize: 24 }} aria-hidden="true">{getTransactionIcon(tx.type)}</div>
                <div style={{ flex: 1 }}>
                  <div style={{ fontSize: 'var(--font-body)', fontWeight: 500 }}>
                    {tx.description || t(`transaction_${tx.type}`)}
                  </div>
                  <div className="text-hint" style={{ fontSize: 11, marginTop: 2 }}>
                    {new Date(tx.created_at).toLocaleDateString()}
                  </div>
                </div>
                <div style={{
                  fontSize: 15,
                  fontWeight: 700,
                  color: getTransactionColor(tx.type),
                }}>
                  {tx.amount > 0 ? '+' : ''}{(tx.amount || 0).toLocaleString()} {wallet.currency}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Wallet Tips */}
      <div style={{ marginTop: 16 }}>
        <h2 style={{ fontSize: 'var(--font-h2)', fontWeight: 'var(--weight-bold)', margin: '0 0 12px' }}>
          {t('wallet_tips_title')}
        </h2>
        <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
          {([1, 2, 3] as const).map(num => (
            <div key={num} className="glass-card" style={{ padding: 16, display: 'flex', gap: 12, alignItems: 'flex-start' }}>
              <div style={{
                width: 32, height: 32, borderRadius: 16,
                background: 'rgba(52, 199, 89, 0.1)', color: 'var(--color-success)',
                display: 'flex', alignItems: 'center', justifyContent: 'center',
                fontSize: 16, fontWeight: 'bold',
                flexShrink: 0,
              }} aria-hidden="true">
                {num === 1 ? '⚡' : num === 2 ? '🚀' : '⏳'}
              </div>
              <div>
                <div style={{ fontWeight: 600, fontSize: 'var(--font-body)', marginBottom: 4 }}>
                  {t(`wallet_tip_${num}_title`)}
                </div>
                <div className="text-hint" style={{ fontSize: 'var(--font-caption)', lineHeight: 1.4 }}>
                  {t(`wallet_tip_${num}_desc`)}
                </div>
              </div>
            </div>
          ))}
        </div>
      </div>

      <div style={{ marginTop: 32, padding: '0 24px', textAlign: 'center', opacity: 0.6 }}>
        <div style={{ fontSize: 'var(--font-caption)', fontWeight: 600, display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 6, marginBottom: 6 }}>
          <span>🛡️</span> {t('no_refund_title')}
        </div>
        <p style={{ fontSize: 11, lineHeight: 1.5, margin: 0 }}>
          {t('no_refund_desc')}
        </p>
      </div>

      <div style={{ height: 32 }} />
    </div>
  );
}
