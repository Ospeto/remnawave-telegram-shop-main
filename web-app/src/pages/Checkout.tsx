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

    const [purchase, setPurchase] = useState<PurchaseResponse | null>(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [uploading, setUploading] = useState(false);
    const [verificationResult, setVerificationResult] = useState<{ status: string, message: string } | null>(null);
    const [phoneCopied, setPhoneCopied] = useState(false);
    const [amountCopied, setAmountCopied] = useState(false);

    const fileInputRef = useRef<HTMLInputElement>(null);

    useEffect(() => {
        if (tg) {
            tg.BackButton.show();
            tg.BackButton.onClick(() => navigate('/plans'));
        }
    }, [tg, navigate]);

    const purchaseCreated = useRef(false);

    useEffect(() => {
        if (!planIndex || !initData || purchaseCreated.current) return;
        purchaseCreated.current = true;

        const body: any = { plan_index: parseInt(planIndex) };
        if (extendKeyId) body.extend_key_id = parseInt(extendKeyId);

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
    }, [planIndex, initData, extendKeyId]);

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

    const copyToClipboard = (text: string, type: 'phone' | 'amount') => {
        navigator.clipboard.writeText(text).then(() => {
            if (type === 'phone') {
                setPhoneCopied(true);
                setTimeout(() => setPhoneCopied(false), 2000);
            } else {
                setAmountCopied(true);
                setTimeout(() => setAmountCopied(false), 2000);
            }
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

                <div className="tip-box tip-box-success" style={{ marginTop: 8 }}>
                    <span className="tip-icon">💡</span>
                    <span>
                        {extendKeyId ? t('success_tip_extend') : t('success_tip_new')}
                    </span>
                </div>

                <button className="btn-primary" onClick={() => navigate('/')} style={{ marginTop: 16 }}>
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
                        <div
                            onClick={() => copyToClipboard(String(purchase?.amount || ''), 'amount')}
                            style={{
                                marginTop: 6, padding: '10px 14px', borderRadius: 10,
                                background: 'rgba(94, 187, 255, 0.06)', border: '1px solid rgba(94, 187, 255, 0.15)',
                                display: 'flex', justifyContent: 'space-between', alignItems: 'center',
                                cursor: 'pointer'
                            }}
                        >
                            <div>
                                <div className="text-link" style={{ fontSize: 24, fontWeight: 700 }}>
                                    {purchase?.amount?.toLocaleString()} {purchase?.currency}
                                </div>
                            </div>
                            <span style={{ fontSize: 12, color: amountCopied ? '#34c759' : 'var(--tg-hint)' }}>
                                {amountCopied ? t('copied') : t('tap_to_copy')}
                            </span>
                        </div>
                    </div>
                </div>

                {/* Step 3 — To this number */}
                <div className="step-row">
                    <span className="step-number">3</span>
                    <div className="step-text" style={{ flex: 1 }}>
                        <strong>{t('guide_step_3')}</strong>
                        <div
                            onClick={() => copyToClipboard(purchase?.payment_phone || '', 'phone')}
                            style={{
                                marginTop: 6, padding: '10px 14px', borderRadius: 10,
                                background: 'rgba(255, 255, 255, 0.03)', border: '1px solid rgba(255, 255, 255, 0.08)',
                                display: 'flex', justifyContent: 'space-between', alignItems: 'center',
                                cursor: 'pointer'
                            }}
                        >
                            <div style={{ fontSize: 18, fontWeight: 700, fontFamily: 'monospace' }}>
                                {purchase?.payment_phone}
                            </div>
                            <span style={{ fontSize: 12, color: phoneCopied ? '#34c759' : 'var(--tg-hint)' }}>
                                {phoneCopied ? t('copied') : t('tap_to_copy')}
                            </span>
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
                <div style={{ display: 'flex', gap: 6, justifyContent: 'center', marginTop: 12 }}>
                    {['KPay', 'Wave', 'AYA Pay'].map(m => (
                        <span key={m} style={{
                            padding: '4px 12px', borderRadius: 20, fontSize: 12,
                            background: 'rgba(255, 255, 255, 0.05)', border: '1px solid rgba(255, 255, 255, 0.08)'
                        }}>{m}</span>
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
