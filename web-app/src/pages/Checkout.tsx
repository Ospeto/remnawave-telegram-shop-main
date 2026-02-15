import { useEffect, useState, useRef } from 'react';
import { useTelegram } from '../lib/twa';
import { useParams, useNavigate, useSearchParams } from 'react-router-dom';

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
    const navigate = useNavigate();
    const [searchParams] = useSearchParams();

    const extendKeyId = searchParams.get('extend');

    const [purchase, setPurchase] = useState<PurchaseResponse | null>(null);
    const [loading, setLoading] = useState(true);
    const [error, setError] = useState<string | null>(null);
    const [uploading, setUploading] = useState(false);
    const [verificationResult, setVerificationResult] = useState<{ status: string, message: string } | null>(null);

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

    if (loading) return (
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: '100vh', gap: 12 }}>
            <div className="spinner" />
            <span className="text-hint" style={{ fontSize: 13 }}>Creating purchase...</span>
        </div>
    );

    if (error) return (
        <div style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: '100vh', gap: 16, padding: 24 }}>
            <div style={{ fontSize: 48 }}>❌</div>
            <p style={{ color: '#ff3b30', textAlign: 'center', fontSize: 14 }}>{error}</p>
            <button className="btn-secondary" onClick={() => navigate('/plans')}>Back to Plans</button>
        </div>
    );

    if (verificationResult?.status === 'success') {
        return (
            <div className="animate-slide-up" style={{ display: 'flex', flexDirection: 'column', alignItems: 'center', justifyContent: 'center', height: '100vh', gap: 16, padding: 24, textAlign: 'center' }}>
                <div style={{ fontSize: 64, marginBottom: 8 }}>✅</div>
                <h1 style={{ fontSize: 22, fontWeight: 700, margin: 0 }}>Payment Verified!</h1>
                <p className="text-hint" style={{ margin: 0, fontSize: 14 }}>
                    {extendKeyId ? 'Your key has been extended.' : 'Your new key is being activated.'}
                </p>
                <button className="btn-primary" onClick={() => navigate('/')} style={{ marginTop: 24 }}>
                    Go to My Keys
                </button>
            </div>
        );
    }

    return (
        <div className="animate-fade-in" style={{ padding: 16, display: 'flex', flexDirection: 'column', gap: 16, minHeight: '100vh' }}>
            {/* Step indicator */}
            <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 8, fontSize: 12 }}>
                <span style={{ color: '#34c759' }}>✓ Plan</span>
                <span className="text-hint">→</span>
                <span className="text-link" style={{ fontWeight: 700 }}>Payment</span>
                <span className="text-hint">→</span>
                <span className="text-hint">Verify</span>
            </div>

            {/* Payment card */}
            <div className="glass-card" style={{ padding: 20 }}>
                <h2 style={{ fontSize: 17, fontWeight: 700, margin: '0 0 16px' }}>💳 Payment Details</h2>

                <div style={{ display: 'flex', flexDirection: 'column', gap: 12 }}>
                    {/* Amount */}
                    <div className="glass-card-active" style={{
                        padding: 16, textAlign: 'center', borderRadius: 12,
                        background: 'rgba(94, 187, 255, 0.06)', border: '1px solid rgba(94, 187, 255, 0.15)'
                    }}>
                        <div className="text-hint" style={{ fontSize: 10, textTransform: 'uppercase', letterSpacing: 1, marginBottom: 4 }}>
                            Amount to Send
                        </div>
                        <div className="text-link" style={{ fontSize: 28, fontWeight: 700 }}>
                            {purchase?.amount?.toLocaleString()} {purchase?.currency}
                        </div>
                    </div>

                    {/* Phone */}
                    <div style={{
                        padding: 16, borderRadius: 12,
                        background: 'rgba(255, 255, 255, 0.03)',
                        border: '1px solid rgba(255, 255, 255, 0.06)'
                    }}>
                        <div className="text-hint" style={{ fontSize: 10, textTransform: 'uppercase', letterSpacing: 1, marginBottom: 4 }}>
                            Send to Phone Number
                        </div>
                        <div style={{ fontSize: 18, fontWeight: 700, fontFamily: 'monospace' }}>
                            {purchase?.payment_phone}
                        </div>
                    </div>

                    {/* Accepted methods */}
                    <div style={{ display: 'flex', gap: 6, justifyContent: 'center' }}>
                        {['KPay', 'Wave', 'AYA Pay'].map(m => (
                            <span key={m} style={{
                                padding: '4px 12px', borderRadius: 20, fontSize: 12,
                                background: 'rgba(255, 255, 255, 0.05)', border: '1px solid rgba(255, 255, 255, 0.08)'
                            }}>{m}</span>
                        ))}
                    </div>

                    {/* Warning */}
                    <div style={{
                        padding: 12, borderRadius: 10, textAlign: 'center', fontSize: 13,
                        background: 'rgba(255, 159, 10, 0.08)', border: '1px solid rgba(255, 159, 10, 0.15)',
                        color: '#ff9f0a'
                    }}>
                        ⚠️ <strong>Do NOT</strong> add any note or remark
                    </div>
                </div>
            </div>

            <div style={{ flex: 1 }} />

            {/* Error */}
            {verificationResult?.status === 'failed' && (
                <div style={{
                    padding: 12, borderRadius: 10, textAlign: 'center', fontSize: 13,
                    background: 'rgba(255, 59, 48, 0.08)', border: '1px solid rgba(255, 59, 48, 0.15)',
                    color: '#ff3b30'
                }}>
                    {verificationResult.message}
                </div>
            )}

            {/* Upload */}
            <input type="file" ref={fileInputRef} onChange={handleFileUpload} style={{ display: 'none' }} accept="image/*" />

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
                {uploading ? '🔍 Verifying...' : '📸 Upload Payment Screenshot'}
            </button>
        </div>
    );
}
