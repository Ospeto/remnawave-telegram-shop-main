import { useEffect, useState, useCallback } from 'react';
import { openTelegramShareLink, useTelegram } from '../lib/twa';
import { useNavigate } from 'react-router-dom';
import { useLanguage } from '../lib/LanguageContext';
import { LoadingScreen } from '../components/LoadingScreen';
import { ErrorScreen } from '../components/ErrorScreen';
import { SessionExpiredScreen } from '../components/SessionExpiredScreen';
import { APIError, isAPIStatus } from '../lib/http';
import { clearTelegramSession, fetchJSONWithTelegramAuth } from '../lib/auth';

interface WalletData {
  balance: number;
  currency: string;
  auto_renew: boolean;
  auto_renew_duration: number | null;
  bot_url: string;
  referral_count?: number;
  referral_earned?: number;
  referral_stats_unavailable?: boolean;
  referral_bonus_amount: number;
}

interface Transaction {
  id: number;
  amount: number;
  type: 'topup' | 'purchase' | 'refund';
  description: string;
  created_at: string;
}

interface ReferralItem {
  id: number;
  masked_id: string;
  created_at: string;
  status: 'bonus_received' | 'pending';
}

export function Wallet() {
  const { tg, initData, close } = useTelegram();
  const { t } = useLanguage();
  const navigate = useNavigate();

  const [wallet, setWallet] = useState<WalletData | null>(null);
  const [transactions, setTransactions] = useState<Transaction[]>([]);
  const [referrals, setReferrals] = useState<ReferralItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [referralLoadError, setReferralLoadError] = useState<string | null>(null);
  const [authExpired, setAuthExpired] = useState(false);

  const handleBack = useCallback(() => navigate('/'), [navigate]);
  const referralTotalsUnavailable = Boolean(wallet?.referral_stats_unavailable);
  const loadWalletData = useCallback(async () => {
    if (!initData) {
      setLoading(false);
      return;
    }

    setLoading(true);
    setLoadError(null);
    setReferralLoadError(null);
    setAuthExpired(false);

    try {
      const [walletData, historyData, referralResult] = await Promise.all([
        fetchJSONWithTelegramAuth<WalletData>('/api/wallet', initData),
        fetchJSONWithTelegramAuth<Transaction[]>('/api/wallet/history?limit=10', initData),
        fetchJSONWithTelegramAuth<ReferralItem[]>('/api/referrals', initData)
          .then((data) => ({ ok: true as const, data }))
          .catch((error) => ({ ok: false as const, error })),
      ]);

      setWallet(walletData);
      setTransactions(historyData || []);
      if (referralResult.ok) {
        setReferrals(Array.isArray(referralResult.data) ? referralResult.data : []);
      } else {
        console.warn('Referral load error:', referralResult.error);
        setReferrals([]);
        setReferralLoadError(t('referral_activity_unavailable'));
      }
    } catch (err) {
      console.warn('Wallet load error:', err);
      if (isAPIStatus(err, 401)) {
        clearTelegramSession();
        setAuthExpired(true);
        return;
      }
      if (err instanceof APIError && err.body) {
        setLoadError(err.body);
        return;
      }
      setLoadError(t('wallet_error'));
    } finally {
      setLoading(false);
    }
  }, [initData, t]);

  useEffect(() => {
    if (!tg) return;
    tg.BackButton.show();
    tg.BackButton.onClick(handleBack);
    return () => tg.BackButton.offClick(handleBack);
  }, [tg, handleBack]);

  useEffect(() => {
    void loadWalletData();
  }, [loadWalletData]);

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

  if (!initData) {
    return (
      <div className="screen-center">
        <div style={{ fontSize: 48 }}>📱</div>
        <h1 style={{ fontSize: 20, fontWeight: 700, margin: 0 }}>Wavy Private Server Shop</h1>
        <p className="text-hint" style={{ margin: 0 }}>{t('open_in_tg')}</p>
      </div>
    );
  }

  if (loading) return <LoadingScreen message={t('loading_wallet')} />;

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

  if (loadError || !wallet) {
    return (
      <ErrorScreen
        message={loadError || t('wallet_error')}
        onRetry={() => { void loadWalletData(); }}
        retryLabel={t('retry')}
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

        {/* Top row: chip + WAVY brand */}
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

          {/* WAVY brand — top-right watermark */}
          <div style={{
            fontSize: 22, fontWeight: 900, letterSpacing: '4px',
            textTransform: 'uppercase',
            color: 'rgba(255,255,255,0.12)',
            userSelect: 'none',
          }}>WAVY</div>
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

      </div>

      {/* Referral Card */}
      <div
        className="animate-slide-up"
        style={{
          borderRadius: 16,
          padding: '20px',
          background: 'var(--card-bg)',
          border: '1px solid var(--border-color)',
          boxShadow: '0 4px 16px rgba(0,0,0,0.06)',
          animationDelay: '0.1s'
        }}
      >
        <h2 style={{ fontSize: '17px', fontWeight: 700, margin: '0 0 16px', display: 'flex', alignItems: 'center', gap: 8 }}>
          {t('referral_earnings')}
        </h2>

        <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12, marginBottom: 16 }}>
          <div style={{ background: 'var(--btn-sec-bg)', padding: '12px', borderRadius: 12 }}>
            <div className="text-hint" style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', marginBottom: 4 }}>{t('friends_invited')}</div>
            <div style={{ fontSize: 24, fontWeight: 800, color: 'var(--text-color)' }}>
              {referralTotalsUnavailable ? '—' : (wallet.referral_count ?? 0)}
            </div>
          </div>
          <div style={{ background: 'var(--btn-sec-bg)', padding: '12px', borderRadius: 12 }}>
            <div className="text-hint" style={{ fontSize: 11, fontWeight: 600, textTransform: 'uppercase', marginBottom: 4 }}>{t('total_earned')}</div>
            <div style={{ fontSize: 20, fontWeight: 800, color: 'var(--color-success)', display: 'flex', alignItems: 'baseline', gap: 4 }}>
              {referralTotalsUnavailable ? '—' : `+${(wallet.referral_earned || 0).toLocaleString()}`} <span style={{ fontSize: 12 }}>{wallet?.currency}</span>
            </div>
          </div>
        </div>

        {referralTotalsUnavailable && (
          <div
            role="status"
            style={{
              marginBottom: 16,
              padding: '10px 12px',
              borderRadius: 10,
              background: 'rgba(255, 159, 10, 0.12)',
              color: 'var(--text-color)',
              fontSize: 12,
              lineHeight: 1.5,
            }}
          >
            {t('referral_totals_unavailable')}
          </div>
        )}

        {referralLoadError && (
          <div
            role="status"
            style={{
              marginBottom: 16,
              padding: '10px 12px',
              borderRadius: 10,
              background: 'rgba(255, 159, 10, 0.12)',
              color: 'var(--text-color)',
              fontSize: 12,
              lineHeight: 1.5,
            }}
          >
            {referralLoadError}
          </div>
        )}

        {
          !referralLoadError && referrals.length > 0 && (
            <div style={{ display: 'flex', flexDirection: 'column', gap: 6, marginBottom: 16 }}>
              {referrals.map(ref => (
                <div key={ref.id} style={{
                  display: 'flex', justifyContent: 'space-between', alignItems: 'center',
                  padding: '10px 12px', borderRadius: 8, background: 'var(--main-bg)'
                }}>
                  <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
                    <div style={{ fontSize: 16, opacity: 0.8 }}>👤</div>
                    <div>
                      <div style={{ fontSize: 13, fontWeight: 600, fontFamily: 'monospace' }}>#{ref.masked_id}</div>
                      <div className="text-hint" style={{ fontSize: 11 }}>{new Date(ref.created_at).toLocaleDateString()}</div>
                    </div>
                  </div>
                  <div style={{
                    fontSize: 12, fontWeight: 600,
                    color: ref.status === 'bonus_received' ? 'var(--color-success)' : 'var(--hint-color)',
                    background: ref.status === 'bonus_received' ? 'rgba(52, 199, 89, 0.1)' : 'rgba(150, 150, 150, 0.1)',
                    padding: '4px 8px', borderRadius: 6
                  }}>
                    {ref.status === 'bonus_received' ? `✅ ${t('referral_bonus_received')}` : `⏳ ${t('referral_pending')}`}
                  </div>
                </div>
              ))}
            </div>
          )
        }

        {/* Share Button uses tg API if possible to pick chat, else copies */}
        <button
          onClick={() => {
            const uid = tg?.initDataUnsafe?.user?.id;
            if (!uid) return;
            // Use the bot URL from the backend if available, otherwise just use the web App's URL parameter or fallback username
            let botUrlToUse = "https://t.me/WavyVpnBot"; // absolute fallback
            if (wallet?.bot_url) {
              botUrlToUse = wallet.bot_url;
            }
            const text = t('referral_share_text');
            const url = `${botUrlToUse}?start=ref_${uid}`;
            openTelegramShareLink(tg, url, text);
          }}
          style={{ width: '100%', padding: '12px', borderRadius: 10, background: 'var(--btn-bg)', color: 'var(--btn-text)', border: 'none', fontWeight: 600, fontSize: 15, cursor: 'pointer' }}
        >
          🎁 {t('share_link')}
        </button>
      </div >

      {/* Transaction History */}
      < div >
        <h2 style={{ fontSize: 'var(--font-h2)', fontWeight: 'var(--weight-bold)', margin: '16px 0 12px' }}>
          {t('transaction_history')}
        </h2>

        {
          transactions.length === 0 ? (
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
          )
        }
      </div >

      {/* Wallet Tips */}
      < div style={{ marginTop: 16 }}>
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
      </div >

      <div style={{ marginTop: 32, padding: '0 24px', textAlign: 'center', opacity: 0.6 }}>
        <div style={{ fontSize: 'var(--font-caption)', fontWeight: 600, display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 6, marginBottom: 6 }}>
          <span>🛡️</span> {t('no_refund_title')}
        </div>
        <p style={{ fontSize: 11, lineHeight: 1.5, margin: 0 }}>
          {t('no_refund_desc')}
        </p>
      </div>

      <div style={{ height: 32 }} />
    </div >
  );
}
