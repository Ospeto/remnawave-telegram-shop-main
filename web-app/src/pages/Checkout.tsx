import { useEffect, useState, useRef, useCallback } from 'react';
import { useTelegram } from '../lib/twa';
import { useParams, useNavigate, useSearchParams } from 'react-router-dom';
import { useLanguage } from '../lib/LanguageContext';
import { LoadingScreen } from '../components/LoadingScreen';
import { TipBox } from '../components/TipBox';

interface PurchaseResponse {
    purchase_id: number;
    payment_phone: string;
    amount: number;
    currency: string;
    instructions: string;
    invoice_type: string;
}

export function Checkout() {
    const { planIndex } = useParams();
    const { tg, initData } = useTelegram();
    const { t } = useLanguage();
    const navigate = useNavigate();
    const [searchParams] = useSearchParams();

    const extendKeyId = searchParams.get('extend');
    const promoCode = searchParams.get('promo');
    const isWalletTopup = searchParams.get('walletTopup') === 'true';
    const amountParam = searchParams.get('amount');

    const [purchase, setPurchase] = useState<PurchaseResponse | null>(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [uploading, setUploading] = useState(false);
    const [verificationResult, setVerificationResult] = useState<{ status: string, message: string, happ_link?: string } | null>(null);
    const [phoneCopied, setPhoneCopied] = useState(false);

    // Wallet payment state
    const [walletBalance, setWalletBalance] = useState<number | null>(null);
    const [payingWithWallet, setPayingWithWallet] = useState(false);
    const [walletPayError, setWalletPayError] = useState<string | null>(null);

    const fileInputRef = useRef<HTMLInputElement>(null);
    const purchaseCreated = useRef(false);
    const idempotencyKey = useRef(crypto.randomUUID());

    const handleBack = useCallback(() => {
        navigate('/plans' + (isWalletTopup ? '?walletTopup=true' : ''));
    }, [navigate, isWalletTopup]);

    useEffect(() => {
        if (!tg) return;
        tg.BackButton.show();
        tg.BackButton.onClick(handleBack);
        return () => tg.BackButton.offClick(handleBack);
    }, [tg, handleBack]);

    useEffect(() => {
        if (!planIndex || !initData || purchaseCreated.current) return;
        purchaseCreated.current = true;

        const body: Record<string, unknown> = {
            plan_index: parseInt(planIndex),
            idempotency_key: idempotencyKey.current
        };
        if (extendKeyId) body.extend_key_id = parseInt(extendKeyId);
        if (promoCode) body.promo_code = promoCode;
        if (isWalletTopup) {
            body.payment_method = 'wallet_topup';
            if (amountParam) body.amount = parseInt(amountParam);
        }

        fetch('/api/purchase', {
            method: 'POST',
            headers: {
                'Authorization': `twa ${initData}`,
                'Content-Type': 'application/json'
            },
            body: JSON.stringify(body)
        })
            .then(res => {
                if (!res.ok) return res.text().then(t => { throw new Error(t) });
                return res.json();
            })
            .then(setPurchase)
            .catch(err => setError(err.message))
            .finally(() => setLoading(false));

        // Fetch wallet balance (silent fail — just hides wallet option)
        if (!isWalletTopup) {
            fetch('/api/wallet', { headers: { 'Authorization': `twa ${initData}` } })
                .then(r => r.json())
                .then(data => setWalletBalance(data.balance))
                .catch(() => { });
        }
    }, [planIndex, initData, extendKeyId, promoCode, isWalletTopup, amountParam]);

    const handlePayWithWallet = async () => {
        if (!purchase || payingWithWallet) return;
        setPayingWithWallet(true);
        setWalletPayError(null);
        try {
            const body: Record<string, unknown> = {
                plan_index: parseInt(planIndex || '0'),
                payment_method: 'wallet'
            };
            if (extendKeyId) body.extend_key_id = parseInt(extendKeyId);
            if (promoCode) body.promo_code = promoCode;

            const res = await fetch('/api/purchase', {
                method: 'POST',
                headers: {
                    'Authorization': `twa ${initData}`,
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify(body)
            });

            if (!res.ok) {
                const text = await res.text();
                throw new Error(text);
            }

            await res.json();
            setVerificationResult({ status: 'success', message: t('wallet_pay_success') });
        } catch (err: unknown) {
            const msg = err instanceof Error ? err.message : t('wallet_pay_error');
            setWalletPayError(msg || t('wallet_pay_error'));
        } finally {
            setPayingWithWallet(false);
        }
    };

    const handleFileUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
        const file = e.target.files?.[0];
        if (!file || !purchase) return;

        setUploading(true);
        setVerificationResult(null);
        const formData = new FormData();
        formData.append('file', file);

        try {
            const res = await fetch(`/api/upload_screenshot?id=${purchase.purchase_id}`, {
                method: 'POST',
                headers: { 'Authorization': `twa ${initData}` },
                body: formData
            });
            if (!res.ok) {
                const errText = await res.text();
                throw new Error(errText || `Upload failed (${res.status})`);
            }
            const data = await res.json();
            setVerificationResult(data);
        } catch (err: unknown) {
            const msg = err instanceof Error ? err.message : 'Upload failed. Please try again.';
            setVerificationResult({ status: 'failed', message: msg });
        } finally {
            setUploading(false);
            if (fileInputRef.current) fileInputRef.current.value = '';
        }
    };

    const copyToClipboard = (text: string) => {
        navigator.clipboard.writeText(text).then(() => {
            setPhoneCopied(true);
            setTimeout(() => setPhoneCopied(false), 2000);
        });
    };

    const handleHappLink = (happUrl: string) => {
        const iframe = document.createElement('iframe');
        iframe.style.display = 'none';
        iframe.src = happUrl;
        document.body.appendChild(iframe);
        setTimeout(() => iframe.remove(), 3000);
        const redirectUrl = `${window.location.origin}/redirect.html?url=${encodeURIComponent(happUrl)}`;
        if (tg?.openLink) tg.openLink(redirectUrl);
        else window.open(redirectUrl, '_blank');
    };

    if (loading) return <LoadingScreen message={t('creating_purchase')} />;

    if (error) return (
        <div className="screen-center">
            <div style={{ fontSize: 48 }} aria-hidden="true">❌</div>
            <p style={{ color: 'var(--color-danger)', textAlign: 'center', fontSize: 14 }}>{error}</p>
            <button className="btn-secondary" onClick={() => navigate('/plans')}>{t('back_to_plans')}</button>
        </div>
    );

    if (verificationResult?.status === 'success') {
        return (
            <div className="animate-slide-up screen-center">
                <div style={{ fontSize: 64, marginBottom: 8 }} aria-hidden="true">✅</div>
                <h1 style={{ fontSize: 22, fontWeight: 700, margin: 0 }}>{t('success_title')}</h1>
                <p className="text-hint" style={{ margin: 0, fontSize: 14 }}>
                    {isWalletTopup ? t('success_topup_desc') : (extendKeyId ? t('success_extend') : t('success_new'))}
                </p>

                {verificationResult?.happ_link && (
                    <button
                        className="btn-primary"
                        onClick={() => handleHappLink(verificationResult.happ_link!)}
                        style={{ marginTop: 12, width: '100%', padding: '14px', fontSize: 15, fontWeight: 700, boxShadow: '0 4px 16px rgba(0,122,255,0.3)' }}
                    >
                        {t('btn_open_happ')}
                    </button>
                )}

                {verificationResult?.happ_link && (
                    <p className="text-hint" style={{ margin: '-8px 0 0', fontSize: 11 }}>
                        {t('success_happ_hint')}
                    </p>
                )}

                {!verificationResult?.happ_link && !isWalletTopup && (
                    <TipBox variant="success" icon="✨">
                        {t('check_home_for_key')}
                    </TipBox>
                )}

                <TipBox variant="success" icon="💡">
                    {isWalletTopup ? t('funds_added') : (extendKeyId ? t('success_tip_extend') : t('success_tip_new'))}
                </TipBox>

                <button className="btn-secondary" onClick={() => navigate(isWalletTopup ? '/wallet' : '/')} style={{ width: '100%', opacity: 0.7 }}>
                    {isWalletTopup ? t('back_to_wallet') : t('go_home')}
                </button>
            </div>
        );
    }

    const canPayWithWallet = purchase && walletBalance !== null && walletBalance >= purchase.amount && !isWalletTopup;

    return (
        <div className="animate-fade-in" style={{ padding: 16, display: 'flex', flexDirection: 'column', gap: 16, minHeight: '100vh' }}>
            {/* Step indicator */}
            {!isWalletTopup && (
                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 8, fontSize: 12 }}>
                    <span style={{ color: 'var(--color-success)' }}>✓ {t('nav_plan')}</span>
                    <span className="text-hint" aria-hidden="true">→</span>
                    <span className="text-link" style={{ fontWeight: 700 }}>{t('nav_payment')}</span>
                    <span className="text-hint" aria-hidden="true">→</span>
                    <span className="text-hint">{t('nav_verify')}</span>
                </div>
            )}
            {isWalletTopup && (
                <div style={{ textAlign: 'center', fontSize: 16, fontWeight: 700 }}>
                    {t('title_top_up')}
                </div>
            )}

            {/* Wallet Payment Option */}
            {canPayWithWallet && (
                <div className="glass-card" style={{ padding: 20, border: '1px solid var(--color-success)' }}>
                    <h2 style={{ fontSize: 17, fontWeight: 700, margin: '0 0 8px' }}>{t('pay_with_wallet')}</h2>
                    <div style={{ display: 'flex', justifyContent: 'space-between', fontSize: 13, marginBottom: 12 }}>
                        <span className="text-hint">{t('your_balance')}</span>
                        <span>{walletBalance?.toLocaleString()} {purchase?.currency}</span>
                    </div>
                    {walletPayError && (
                        <div role="alert" style={{
                            padding: 10, borderRadius: 8, marginBottom: 10,
                            background: 'rgba(255, 59, 48, 0.08)', border: '1px solid rgba(255, 59, 48, 0.15)',
                            color: 'var(--color-danger)', fontSize: 13
                        }}>
                            {walletPayError}
                        </div>
                    )}
                    <button
                        className="btn-primary"
                        onClick={handlePayWithWallet}
                        disabled={payingWithWallet}
                        style={{ width: '100%', background: 'var(--color-success)', opacity: payingWithWallet ? 0.7 : 1 }}
                    >
                        {payingWithWallet
                            ? <><div className="spinner" style={{ width: 16, height: 16, borderWidth: 2 }} />{t('wallet_pay_processing')}</>
                            : t('wallet_pay_btn', { amount: (purchase?.amount || 0).toLocaleString(), currency: purchase?.currency || '' })}
                    </button>
                </div>
            )}

            {/* Manual Payment Guide */}
            <div className="glass-card" style={{ padding: 20 }}>
                <h2 style={{ fontSize: 17, fontWeight: 700, margin: '0 0 12px' }}>
                    {canPayWithWallet ? t('or_pay_manually') : t('guide_title')}
                </h2>

                <TipBox variant="info" icon="ℹ️" allowHtml style={{ marginBottom: 16 }}>
                    {purchase?.instructions || ''}
                </TipBox>

                {/* Quick Copy Helpers */}
                <div style={{ display: 'grid', gridTemplateColumns: '1fr 1fr', gap: 12 }}>
                    <div style={{
                        padding: 12, borderRadius: 12,
                        background: 'var(--input-bg)', border: '1px solid var(--input-border)',
                        display: 'flex', flexDirection: 'column', gap: 4
                    }}>
                        <div className="text-hint" style={{ fontSize: 11 }}>Amount</div>
                        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                            <div style={{ fontWeight: 700 }}>{(purchase?.amount || 0).toLocaleString()}</div>
                            <button
                                onClick={() => copyToClipboard(String(purchase?.amount))}
                                className="btn-secondary"
                                style={{ padding: '4px 8px', fontSize: 14, minWidth: 32 }}
                            >
                                📋
                            </button>
                        </div>
                    </div>
                    <div style={{
                        padding: 12, borderRadius: 12,
                        background: 'var(--input-bg)', border: '1px solid var(--input-border)',
                        display: 'flex', flexDirection: 'column', gap: 4
                    }}>
                        <div className="text-hint" style={{ fontSize: 11 }}>Phone</div>
                        <div style={{ display: 'flex', justifyContent: 'space-between', alignItems: 'center' }}>
                            <div style={{ fontWeight: 700, fontFamily: 'monospace', fontSize: 13 }}>{purchase?.payment_phone}</div>
                            <button
                                onClick={() => copyToClipboard(purchase?.payment_phone || '')}
                                className="btn-secondary"
                                style={{ padding: '4px 8px', fontSize: 14, minWidth: 32, color: phoneCopied ? 'var(--color-success)' : undefined }}
                            >
                                {phoneCopied ? '✓' : '📋'}
                            </button>
                        </div>
                    </div>
                </div>
            </div>

            <TipBox variant="warning" icon="⚠️">{t('important_warning')}</TipBox>

            <div style={{ flex: 1 }} />

            {/* Upload verification error */}
            {verificationResult?.status === 'failed' && (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                    <div role="alert" style={{
                        padding: 12, borderRadius: 10, textAlign: 'center', fontSize: 13,
                        background: 'rgba(255, 59, 48, 0.08)', border: '1px solid rgba(255, 59, 48, 0.15)',
                        color: 'var(--color-danger)'
                    }}>
                        ❌ {verificationResult.message}
                    </div>
                    <TipBox variant="info" icon="💡" style={{ fontSize: 11 }}>
                        {t('verify_error_tip')}
                    </TipBox>
                </div>
            )}

            {/* Upload */}
            <input type="file" ref={fileInputRef} onChange={handleFileUpload} style={{ display: 'none' }} accept="image/*" />

            <div style={{ display: 'flex', flexDirection: 'column', gap: 6 }}>
                <button
                    disabled={uploading}
                    onClick={() => fileInputRef.current?.click()}
                    className="btn-primary"
                    style={{
                        fontSize: 17,
                        padding: '16px 24px',
                        opacity: uploading ? 0.6 : 1,
                        cursor: uploading ? 'not-allowed' : 'pointer'
                    }}
                >
                    {uploading
                        ? <><div className="spinner" style={{ width: 18, height: 18, borderWidth: 2 }} />{t('uploading_btn')}</>
                        : t('upload_btn')}
                </button>
                <p className="text-hint" style={{ textAlign: 'center', fontSize: 11, margin: 0 }}>
                    {t('upload_hint')}
                </p>
            </div>
        </div>
    );
}
