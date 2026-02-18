import { useEffect, useState, useRef } from 'react';
import { useTelegram } from '../lib/twa';
import { useParams, useNavigate, useSearchParams } from 'react-router-dom';
import { useLanguage } from '../lib/LanguageContext';

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

    const [purchase, setPurchase] = useState<PurchaseResponse | null>(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [uploading, setUploading] = useState(false);
    const [verificationResult, setVerificationResult] = useState<{ status: string, message: string, happ_link?: string } | null>(null);
    const [phoneCopied, setPhoneCopied] = useState(false);

    const fileInputRef = useRef<HTMLInputElement>(null);

    useEffect(() => {
        if (tg) {
            tg.BackButton.show();
            // Preserve flow: if from plans, go back to plans.
            tg.BackButton.onClick(() => navigate('/plans'));
        }
    }, [tg, navigate]);

    const purchaseCreated = useRef(false);

    useEffect(() => {
        if (!planIndex || !initData || purchaseCreated.current) return;
        purchaseCreated.current = true;

        const body: any = { plan_index: parseInt(planIndex) };
        if (extendKeyId) body.extend_key_id = parseInt(extendKeyId);
        if (promoCode) body.promo_code = promoCode;

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
    }, [planIndex, initData, extendKeyId, promoCode]);

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
        } catch (err: any) {
            setVerificationResult({ status: 'failed', message: err?.message || 'Upload failed. Please try again.' });
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

    if (loading) return (
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: '100vh', gap: 12 }}>
            <div className="spinner" />
            <span className="text-hint" style={{ fontSize: 13 }}>{t('creating_purchase')}</span>
        </div>
    );

    if (error) return (
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: '100vh', gap: 16, padding: 24 }}>
            <div style={{ fontSize: 48 }}>❌</div>
            <p style={{ color: '#ff3b30', textAlign: 'center', fontSize: 14 }}>{error}</p>
            <button className="btn-secondary" onClick={() => navigate('/plans')}>{t('back_to_plans')}</button>
        </div>
    );

    if (verificationResult?.status === 'success') {
        return (
            <div className="animate-slide-up" style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: '100vh', gap: 16, padding: 24, textAlign: 'center' }}>
                <div style={{ fontSize: 64, marginBottom: 8 }}>✅</div>
                <h1 style={{ fontSize: 22, fontWeight: 700, margin: 0 }}>{t('success_title')}</h1>
                <p className="text-hint" style={{ margin: 0, fontSize: 14 }}>
                    {extendKeyId ? t('success_extend') : t('success_new')}
                </p>

                {/* Primary: Open in Happ */}
                <button
                    className="btn-primary"
                    onClick={() => {
                        if (verificationResult?.happ_link) {
                            const happUrl = verificationResult.happ_link;
                            // Strategy 1: Try hidden iframe (works on most Android/iOS)
                            const iframe = document.createElement('iframe');
                            iframe.style.display = 'none';
                            iframe.src = happUrl;
                            document.body.appendChild(iframe);
                            setTimeout(() => iframe.remove(), 3000);
                            // Strategy 2: Also try tg.openLink with redirect.html
                            const redirectUrl = `${window.location.origin}/redirect.html?url=${encodeURIComponent(happUrl)}`;
                            if (tg?.openLink) {
                                tg.openLink(redirectUrl);
                            } else {
                                window.open(redirectUrl, '_blank');
                            }
                        } else {
                            navigate('/');
                        }
                    }}
                    style={{ marginTop: 12, width: '100%', padding: '14px', fontSize: 15, fontWeight: 700, boxShadow: '0 4px 16px rgba(0,122,255,0.3)' }}
                >
                    {t('btn_open_happ')}
                </button>
                <p className="text-hint" style={{ margin: '-8px 0 0', fontSize: 11 }}>
                    {t('success_happ_hint')}
                </p>

                <div className="tip-box tip-box-success" style={{ marginTop: 4 }}>
                    <span className="tip-icon">💡</span>
                    <span>
                        {extendKeyId ? t('success_tip_extend') : t('success_tip_new')}
                    </span>
                </div>

                {/* Secondary: Go to My Keys */}
                <button className="btn-secondary" onClick={() => navigate('/')} style={{ width: '100%', opacity: 0.7 }}>
                    {t('go_home')}
                </button>
            </div>
        );
    }

    return (
        <div className="animate-fade-in" style={{ padding: 16, display: 'flex', flexDirection: 'column', gap: 16, minHeight: '100vh' }}>
            {/* Step indicator */}
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 8, fontSize: 12 }}>
                <span style={{ color: '#34c759' }}>✓ {t('nav_plan')}</span>
                <span className="text-hint">→</span>
                <span className="text-link" style={{ fontWeight: 700 }}>{t('nav_payment')}</span>
                <span className="text-hint">→</span>
                <span className="text-hint">{t('nav_verify')}</span>
            </div>

            {/* How to pay — step by step */}
            <div className="glass-card" style={{ padding: 20 }}>
                <h2 style={{ fontSize: 17, fontWeight: 700, margin: '0 0 12px' }}>{t('guide_title')}</h2>

                {/* Step 1 — Open banking app */}
                <div className="step-row">
                    <span className="step-number">1</span>
                    <div className="step-text">
                        <strong>{t('guide_step_1')}</strong>
                        <div className="text-hint" style={{ fontSize: 11, marginTop: 2 }}>{t('guide_step_1_hint')}</div>
                    </div>
                </div>

                {/* Step 2 — Send exact amount */}
                <div className="step-row">
                    <span className="step-number">2</span>
                    <div className="step-text" style={{ flex: 1 }}>
                        <strong>{t('guide_step_2')}</strong>
                        <div style={{ display: 'flex', gap: 8, marginTop: 6 }}>
                            <div style={{
                                flex: 1, padding: '12px 14px', borderRadius: 12,
                                background: 'rgba(94, 187, 255, 0.06)', border: '1px solid rgba(94, 187, 255, 0.15)',
                                display: 'flex', alignItems: 'center'
                            }}>
                                <div className="text-link" style={{ fontSize: 24, fontWeight: 700 }}>
                                    {purchase?.amount?.toLocaleString()} {purchase?.currency}
                                </div>
                            </div>
                        </div>
                    </div>
                </div>

                {/* Step 3 — To this number */}
                <div className="step-row">
                    <span className="step-number">3</span>
                    <div className="step-text" style={{ flex: 1 }}>
                        <strong>{t('guide_step_3')}</strong>
                        <div style={{ display: 'flex', gap: 8, marginTop: 6 }}>
                            <div style={{
                                flex: 1, padding: '12px 14px', borderRadius: 12,
                                background: 'rgba(255, 255, 255, 0.03)', border: '1px solid rgba(255, 255, 255, 0.08)',
                                display: 'flex', alignItems: 'center'
                            }}>
                                <div style={{ fontSize: 18, fontWeight: 700, fontFamily: 'monospace' }}>
                                    {purchase?.payment_phone}
                                </div>
                            </div>
                            <button
                                onClick={() => copyToClipboard(purchase?.payment_phone || '')}
                                className="btn-secondary"
                                style={{
                                    width: 'auto', padding: '0 16px', borderRadius: 12,
                                    fontSize: 18,
                                    background: phoneCopied ? '#34c759' : undefined,
                                    color: phoneCopied ? '#fff' : undefined,
                                    border: phoneCopied ? 'none' : undefined
                                }}
                            >
                                {phoneCopied ? '✓' : '📋'}
                            </button>
                        </div>
                    </div>
                </div>

                {/* Step 4 — No notes */}
                <div className="step-row">
                    <span className="step-number">4</span>
                    <div className="step-text">
                        <strong>{t('guide_step_4')}</strong>
                        <div className="text-hint" style={{ fontSize: 11, marginTop: 2 }}>{t('guide_step_4_hint')}</div>
                    </div>
                </div>

                {/* Accepted methods badges */}
                <div style={{ display: 'flex', gap: 8, justifyContent: 'center', marginTop: 12 }}>
                    {[
                        {
                            name: 'KPay',
                            color: '#0057b8',
                            ios: 'kbzpay://',
                            android: 'intent://#Intent;scheme=kbzpay;package=com.kbzbank.kpaywallet;S.browser_fallback_url=https://www.kbzpay.com;end',
                            web: 'https://www.kbzpay.com'
                        },
                        {
                            name: 'Wave',
                            color: '#ffd100',
                            txColor: '#000',
                            ios: 'wavemoney://',
                            android: 'intent://#Intent;scheme=wavemoney;package=com.wavemoney.consumer;S.browser_fallback_url=https://www.wavemoney.com.mm;end',
                            web: 'https://www.wavemoney.com.mm'
                        },
                        {
                            name: 'AYA Pay',
                            color: '#ed1c24',
                            ios: 'ayapay://',
                            android: 'intent://#Intent;scheme=ayapay;package=com.ayaplus.wallet;S.browser_fallback_url=https://www.ayapay.com;end',
                            web: 'https://www.ayapay.com'
                        }
                    ].map(m => (
                        <button
                            key={m.name}
                            style={{
                                padding: '6px 14px', borderRadius: 20, fontSize: 12,
                                background: m.color, color: m.txColor || '#fff',
                                fontWeight: 700, opacity: 0.9, transition: 'transform 0.1s',
                                border: 'none', cursor: 'pointer'
                            }}
                            onClick={() => {
                                const userAgent = navigator.userAgent || navigator.vendor || (window as any).opera;
                                let url = m.web;

                                if (/android/i.test(userAgent)) {
                                    url = m.android;
                                } else if (/iPad|iPhone|iPod/.test(userAgent) && !(window as any).MSStream) {
                                    url = m.ios;
                                }

                                window.open(url, '_blank');
                            }}
                        >
                            {m.name} ↗
                        </button>
                    ))}
                </div>
            </div>

            {/* Important warnings */}
            <div className="tip-box tip-box-warning">
                <span className="tip-icon">⚠️</span>
                <span>{t('important_warning')}</span>
            </div>

            <div style={{ flex: 1 }} />

            {/* Error */}
            {verificationResult?.status === 'failed' && (
                <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                    <div style={{
                        padding: 12, borderRadius: 10, textAlign: 'center', fontSize: 13,
                        background: 'rgba(255, 59, 48, 0.08)', border: '1px solid rgba(255, 59, 48, 0.15)',
                        color: '#ff3b30'
                    }}>
                        ❌ {verificationResult.message}
                    </div>
                    <div className="tip-box tip-box-info" style={{ fontSize: 11 }}>
                        <span className="tip-icon">💡</span>
                        <span>{t('verify_error_tip')}</span>
                    </div>
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
                    {uploading ? t('uploading_btn') : t('upload_btn')}
                </button>
                <p className="text-hint" style={{ textAlign: 'center', fontSize: 11, margin: 0 }}>
                    {t('upload_hint')}
                </p>
            </div>
        </div>
    );
}
