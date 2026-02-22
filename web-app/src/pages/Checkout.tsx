import { useEffect, useState, useRef, useCallback } from 'react';
import { useTelegram } from '../lib/twa';
import { useParams, useNavigate, useSearchParams } from 'react-router-dom';
import { useLanguage } from '../lib/LanguageContext';
import { LoadingScreen } from '../components/LoadingScreen';
import { TipBox } from '../components/TipBox';
import { useMXBrownSound } from '../lib/useMXBrownSound';

interface PurchaseResponse {
    purchase_id: number;
    payment_phone: string;
    amount: number;
    currency: string;
    instructions: string;
    invoice_type: string;
    bot_url: string;
}

export function Checkout() {
    const { planIndex } = useParams();
    const { tg, initData } = useTelegram();
    const { t } = useLanguage();
    const { playClick } = useMXBrownSound();
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
        navigator.clipboard.writeText(text)
            .then(() => {
                setPhoneCopied(true);
                setTimeout(() => setPhoneCopied(false), 2000);
            })
            .catch(() => {
                // Fallback for environments where clipboard API is unavailable
                const textArea = document.createElement('textarea');
                textArea.value = text;
                textArea.style.position = 'fixed';
                textArea.style.left = '-9999px';
                document.body.appendChild(textArea);
                textArea.select();
                document.execCommand('copy');
                document.body.removeChild(textArea);
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
            <button className="btn-secondary" onClick={() => { playClick(); navigate('/plans'); }}>{t('back_to_plans')}</button>
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
                        onClick={() => { playClick(); handleHappLink(verificationResult.happ_link!); }}
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

                {/* Referral CTA on Success */}
                <div style={{
                    marginTop: 24, marginBottom: 24,
                    padding: '16px 20px', borderRadius: 16,
                    background: 'linear-gradient(135deg, rgba(201,168,76,0.1) 0%, rgba(184,144,42,0.1) 100%)',
                    border: '1px solid rgba(201,168,76,0.25)',
                    textAlign: 'center'
                }}>
                    <div style={{ fontSize: 16, fontWeight: 700, color: 'var(--text-color)', marginBottom: 6 }}>
                        {t('referral_checkout_title')}
                    </div>
                    <div className="text-hint" style={{ fontSize: 13, marginBottom: 16, lineHeight: 1.4 }}>
                        {t('referral_checkout_desc')}
                    </div>
                    <button
                        className="btn-primary"
                        onClick={() => {
                            playClick();
                            const uid = tg?.initDataUnsafe?.user?.id;
                            if (!uid) return;
                            let botUrlToUse = "https://t.me/WavyVpnBot";
                            if (purchase?.bot_url) {
                                botUrlToUse = purchase.bot_url;
                            }
                            const text = `Hey! Join Wavy Private Server using my link and we both get free VPN balance! 🌊`;
                            const url = `${botUrlToUse}?start=ref_${uid}`;
                            (tg as any).openTelegramLink(`https://t.me/share/url?url=${encodeURIComponent(url)}&text=${encodeURIComponent(text)}`);
                        }}
                        style={{
                            width: '100%', padding: '12px', fontSize: 14, fontWeight: 700,
                            background: 'linear-gradient(135deg, #c9a84c 0%, #b8902a 100%)',
                            color: '#000', border: 'none', boxShadow: '0 4px 12px rgba(201,168,76,0.3)'
                        }}
                    >
                        {t('referral_checkout_btn')}
                    </button>
                </div>

                <button className="btn-secondary" onClick={() => { playClick(); navigate(isWalletTopup ? '/wallet' : '/'); }} style={{ width: '100%', opacity: 0.7 }}>
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
                        onClick={() => { playClick(); handlePayWithWallet(); }}
                        disabled={payingWithWallet}
                        style={{ width: '100%', background: 'var(--color-success)', opacity: payingWithWallet ? 0.7 : 1 }}
                    >
                        {payingWithWallet
                            ? <><div className="spinner" style={{ width: 16, height: 16, borderWidth: 2 }} />{t('wallet_pay_processing')}</>
                            : t('wallet_pay_btn', { amount: (purchase?.amount || 0).toLocaleString(), currency: purchase?.currency || '' })}
                    </button>
                </div>
            )}

            {/* Manual Payment Guide — numbered steps */}
            <div className="glass-card" style={{ padding: 20 }}>
                <h2 style={{ fontSize: 17, fontWeight: 700, margin: '0 0 20px' }}>
                    {canPayWithWallet ? t('or_pay_manually') : t('guide_title')}
                </h2>

                {/* Step 1 */}
                <div style={{ display: 'flex', alignItems: 'flex-start', gap: 12, marginBottom: 18 }}>
                    <div style={{
                        minWidth: 28, height: 28, borderRadius: '50%',
                        background: 'var(--tg-btn)', color: 'var(--tg-btn-text)',
                        display: 'flex', alignItems: 'center', justifyContent: 'center',
                        fontSize: 13, fontWeight: 700, flexShrink: 0
                    }}>1</div>
                    <div>
                        <div style={{ fontWeight: 600, fontSize: 14 }}>{t('guide_step_1')}</div>
                        <div className="text-hint" style={{ fontSize: 12, marginTop: 2 }}>{t('guide_step_1_hint')}</div>
                    </div>
                </div>

                {/* Step 2 — amount + phone cards */}
                <div style={{ display: 'flex', alignItems: 'flex-start', gap: 12, marginBottom: 18 }}>
                    <div style={{
                        minWidth: 28, height: 28, borderRadius: '50%',
                        background: 'var(--tg-btn)', color: 'var(--tg-btn-text)',
                        display: 'flex', alignItems: 'center', justifyContent: 'center',
                        fontSize: 13, fontWeight: 700, flexShrink: 0
                    }}>2</div>
                    <div style={{ flex: 1 }}>
                        <div style={{ fontWeight: 600, fontSize: 14, marginBottom: 10 }}>{t('guide_step_2')}</div>
                        <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
                            {/* Amount — plain, no card */}
                            <div style={{ display: 'flex', alignItems: 'baseline', gap: 6 }}>
                                <span className="text-hint" style={{ fontSize: 12 }}>{t('label_amount')}:</span>
                                <span style={{ fontWeight: 800, fontSize: 22, letterSpacing: '-0.5px' }}>{(purchase?.amount || 0).toLocaleString()}</span>
                                <span className="text-hint" style={{ fontSize: 13 }}>{purchase?.currency}</span>
                            </div>
                            {/* Phone — card with big font + copy */}
                            <div style={{ padding: '10px 12px', borderRadius: 12, background: 'var(--input-bg)', border: '1px solid var(--input-border)' }}>
                                <div className="text-hint" style={{ fontSize: 11, marginBottom: 4 }}>{t('label_phone')}</div>
                                <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 4 }}>
                                    <div style={{ fontWeight: 700, fontSize: 18, fontFamily: 'monospace', letterSpacing: '0.5px' }}>{purchase?.payment_phone}</div>
                                    <button onClick={() => { playClick(); copyToClipboard(purchase?.payment_phone || ''); }} className="btn-secondary" aria-label={t('tap_to_copy')} style={{ padding: '4px 8px', fontSize: 13, minWidth: 30, borderRadius: 8, color: phoneCopied ? 'var(--color-success)' : undefined }}>{phoneCopied ? '✓' : '📋'}</button>
                                </div>
                            </div>
                        </div>
                    </div>
                </div>

                {/* Step 3 — remark */}
                <div style={{ display: 'flex', alignItems: 'flex-start', gap: 12, marginBottom: 18 }}>
                    <div style={{
                        minWidth: 28, height: 28, borderRadius: '50%',
                        background: 'var(--tg-btn)', color: 'var(--tg-btn-text)',
                        display: 'flex', alignItems: 'center', justifyContent: 'center',
                        fontSize: 13, fontWeight: 700, flexShrink: 0
                    }}>3</div>
                    <div>
                        <div style={{ fontWeight: 600, fontSize: 14 }}>{t('guide_step_3')}</div>
                        <div className="text-hint" style={{ fontSize: 12, marginTop: 2 }}>{t('guide_step_3_hint')}</div>
                    </div>
                </div>

                {/* Step 4 — screenshot */}
                <div style={{ display: 'flex', alignItems: 'flex-start', gap: 12 }}>
                    <div style={{
                        minWidth: 28, height: 28, borderRadius: '50%',
                        background: 'var(--tg-btn)', color: 'var(--tg-btn-text)',
                        display: 'flex', alignItems: 'center', justifyContent: 'center',
                        fontSize: 13, fontWeight: 700, flexShrink: 0
                    }}>4</div>
                    <div>
                        <div style={{ fontWeight: 600, fontSize: 14 }}>{t('guide_step_4')}</div>
                        <div className="text-hint" style={{ fontSize: 12, marginTop: 2 }}>{t('guide_step_4_hint')}</div>
                    </div>
                </div>
            </div>

            <div style={{ flex: 1 }} />

            {/* Upload verification error */}
            {verificationResult?.status === 'failed' && (
                <div role="alert" style={{
                    padding: 12, borderRadius: 12, fontSize: 13, textAlign: 'center',
                    background: 'rgba(255, 59, 48, 0.08)', border: '1px solid rgba(255, 59, 48, 0.18)',
                    color: 'var(--color-danger)'
                }}>
                    ❌ {verificationResult.message}
                    <div className="text-hint" style={{ fontSize: 11, marginTop: 6 }}>{t('verify_error_tip')}</div>
                </div>
            )}

            {/* Upload */}
            <input type="file" ref={fileInputRef} onChange={handleFileUpload} style={{ display: 'none' }} accept="image/*" />
            <button
                disabled={uploading}
                onClick={() => { playClick(); fileInputRef.current?.click(); }}
                className="btn-primary"
                style={{ fontSize: 16, padding: '16px 24px', opacity: uploading ? 0.6 : 1, cursor: uploading ? 'not-allowed' : 'pointer' }}
            >
                {uploading
                    ? <><div className="spinner" style={{ width: 18, height: 18, borderWidth: 2 }} />{t('uploading_btn')}</>
                    : t('upload_btn')}
            </button>
        </div>
    );
}
