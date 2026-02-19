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

      {/* Balance Card - Premium Digital Card */}
      <div className="digital-card animate-slide-up" style={{ padding: '24px', textAlign: 'center', position: 'relative' }}>
        {/* Decorative card chip icon */}
        <div style={{
          position: 'absolute', top: 20, left: 24,
          width: 40, height: 28, borderRadius: 6,
          background: 'var(--digital-card-chip-bg)',
          border: '1px solid var(--digital-card-chip-border)'
        }} aria-hidden="true" />

        {/* Visa-style contactless icon */}
        <div style={{
          position: 'absolute', top: 20, right: 24,
          fontSize: 24, opacity: 0.8
        }} aria-hidden="true">
          <svg width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2">
            <path d="M5 12.55a11 11 0 0 1 14.08 0" />
            <path d="M1.42 9a16 16 0 0 1 21.16 0" />
            <path d="M8.53 16.11a6 6 0 0 1 6.95 0" />
            <line x1="12" y1="20" x2="12.01" y2="20" />
          </svg>
        </div>

        <div className="text-hint" style={{ fontSize: '13px', marginBottom: 8, color: 'var(--digital-card-hint)', marginTop: 20 }}>
          {t('current_balance')}
        </div>
        <div style={{ fontSize: '36px', fontWeight: '800', marginBottom: 24, textShadow: '0 2px 10px rgba(0,0,0,0.2)' }}>
          {(wallet?.balance || 0).toLocaleString()} {wallet?.currency || ''}
        </div>
        <button
          className="btn-primary"
          style={{
            marginTop: 0, width: '100%',
            background: 'var(--digital-card-inner-bg)',
            backdropFilter: 'blur(10px)',
            border: '1px solid var(--digital-card-inner-border)',
            boxShadow: 'none'
          }}
          onClick={() => navigate('/plans?walletTopup=true')}
        >
          {t('top_up_wallet')}
        </button>
      </div>


      {/* Transaction History */}
      <div>
        <h2 style={{ fontSize: 'var(--font-h2)', fontWeight: 'var(--weight-bold)', margin: '16px 0 12px' }}>
          {t('transaction_history')}
        </h2>

        {transactions.length === 0 ? (
          <div className="glass-card" style={{ padding: 24, textAlign: 'center' }}>
            <div style={{ fontSize: 32, marginBottom: 8 }} aria-hidden="true">📭</div>
            <div style={{ fontWeight: 600, fontSize: 'var(--font-h2)', marginBottom: 4 }}>{t('wallet_empty_title')}</div>
            <div className="text-hint">{t('wallet_empty_desc')}</div>
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
