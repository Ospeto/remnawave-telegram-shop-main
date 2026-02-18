import { useEffect, useState } from 'react';
import { useTelegram } from '../lib/twa';
import { useNavigate } from 'react-router-dom';
import { useLanguage } from '../lib/LanguageContext';

interface WalletData {
  balance: number;
  currency: string;
  auto_renew: boolean;
  auto_renew_duration: number;
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
  const [updatingAutoRenew, setUpdatingAutoRenew] = useState(false);

  useEffect(() => {
    if (tg) {
      tg.BackButton.show();
      tg.BackButton.onClick(() => navigate('/'));
    }
  }, [tg, navigate]);

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
      .catch(console.error)
      .finally(() => setLoading(false));
  }, [initData]);

  const toggleAutoRenew = async () => {
    if (!wallet || updatingAutoRenew) return;
    
    setUpdatingAutoRenew(true);
    try {
      const res = await fetch('/api/wallet/autorenew', {
        method: 'POST',
        headers: {
          'Authorization': `twa ${initData}`,
          'Content-Type': 'application/json',
        },
        body: JSON.stringify({
          enabled: !wallet.auto_renew,
          duration: wallet.auto_renew_duration,
        }),
      });
      
      if (res.ok) {
        setWallet({ ...wallet, auto_renew: !wallet.auto_renew });
      }
    } catch (err) {
      console.error('Failed to update auto-renew:', err);
    } finally {
      setUpdatingAutoRenew(false);
    }
  };

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

  if (loading) return (
    <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: '100vh', gap: 12 }}>
      <div className="spinner" />
      <span className="text-hint" style={{ fontSize: 13 }}>{t('loading_wallet')}</span>
    </div>
  );

  if (!wallet) return (
    <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: '100vh', gap: 16, padding: 24 }}>
      <div style={{ fontSize: 48 }}>❌</div>
      <p style={{ color: '#ff3b30', textAlign: 'center', fontSize: 14 }}>{t('wallet_error')}</p>
      <button className="btn-secondary" onClick={() => navigate('/')}>{t('go_home')}</button>
    </div>
  );

  return (
    <div className="animate-fade-in" style={{ padding: 16, display: 'flex', flexDirection: 'column', gap: 16, minHeight: '100vh' }}>
      {/* Header */}
      <div style={{ textAlign: 'center', padding: '8px 0' }}>
        <h1 style={{ fontSize: 20, fontWeight: 700, margin: 0 }}>{t('wallet_title')}</h1>
        <p className="text-hint" style={{ fontSize: 12, margin: '6px 0 0' }}>
          {t('wallet_subtitle')}
        </p>
      </div>

      {/* Balance Card */}
      <div className="glass-card" style={{ padding: 24, textAlign: 'center' }}>
        <div className="text-hint" style={{ fontSize: 13, marginBottom: 8 }}>
          {t('current_balance')}
        </div>
        <div style={{ fontSize: 36, fontWeight: 800, marginBottom: 4 }}>
          {wallet.balance.toLocaleString()} {wallet.currency}
        </div>
        <button 
          className="btn-primary"
          style={{ marginTop: 16, width: '100%' }}
          onClick={() => navigate('/plans?walletTopup=true')}
        >
          {t('top_up_wallet')}
        </button>
      </div>

      {/* Auto-Renew Toggle */}
      <div className="glass-card" style={{ padding: 16 }}>
        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between' }}>
          <div>
            <div style={{ fontWeight: 600, fontSize: 15, marginBottom: 4 }}>
              {t('auto_renew_title')}
            </div>
            <div className="text-hint" style={{ fontSize: 12 }}>
              {wallet.auto_renew 
                ? t('auto_renew_enabled', { days: wallet.auto_renew_duration })
                : t('auto_renew_disabled')
              }
            </div>
          </div>
          <button
            onClick={toggleAutoRenew}
            disabled={updatingAutoRenew}
            style={{
              width: 52,
              height: 32,
              borderRadius: 16,
              border: 'none',
              background: wallet.auto_renew ? '#34c759' : '#ccc',
              position: 'relative',
              cursor: updatingAutoRenew ? 'not-allowed' : 'pointer',
              transition: 'background 0.2s',
              opacity: updatingAutoRenew ? 0.7 : 1,
            }}
          >
            <div style={{
              width: 26,
              height: 26,
              borderRadius: 13,
              background: '#fff',
              position: 'absolute',
              top: 3,
              left: wallet.auto_renew ? 23 : 3,
              transition: 'left 0.2s',
              boxShadow: '0 2px 4px rgba(0,0,0,0.2)',
            }} />
          </button>
        </div>
      </div>

      {/* Transaction History */}
      <div>
        <h2 style={{ fontSize: 16, fontWeight: 700, margin: '16px 0 12px' }}>
          {t('transaction_history')}
        </h2>
        
        {transactions.length === 0 ? (
          <div className="glass-card" style={{ padding: 24, textAlign: 'center' }}>
            <div style={{ fontSize: 32, marginBottom: 8 }}>📭</div>
            <div className="text-hint">{t('no_transactions')}</div>
          </div>
        ) : (
          <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
            {transactions.map((tx) => (
              <div key={tx.id} className="glass-card" style={{ padding: 14, display: 'flex', alignItems: 'center', gap: 12 }}>
                <div style={{ fontSize: 24 }}>{getTransactionIcon(tx.type)}</div>
                <div style={{ flex: 1 }}>
                  <div style={{ fontSize: 14, fontWeight: 500 }}>
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
                  {tx.amount > 0 ? '+' : ''}{tx.amount.toLocaleString()} {wallet.currency}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      {/* Info Box */}
      <div className="tip-box tip-box-info" style={{ marginTop: 'auto' }}>
        <span className="tip-icon">💡</span>
        <span>{t('wallet_info')}</span>
      </div>
    </div>
  );
}
