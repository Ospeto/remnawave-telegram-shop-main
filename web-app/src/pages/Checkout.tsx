import { useEffect, useState, useRef } from 'react';
import { useTelegram } from '../lib/twa';
import { useParams, useNavigate } from 'react-router-dom';

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
        return () => { };
    }, [tg, navigate]);

    const purchaseCreated = useRef(false);

    useEffect(() => {
        if (!planIndex || !initData || purchaseCreated.current) return;
        purchaseCreated.current = true;

        fetch('/api/purchase', {
            method: 'POST',
            headers: {
                'Authorization': `twa ${initData}`,
                'Content-Type': 'application/json'
            },
            body: JSON.stringify({ plan_index: parseInt(planIndex) })
        })
            .then(res => {
                if (!res.ok) return res.text().then(t => { throw new Error(t) });
                return res.json();
            })
            .then(setPurchase)
            .catch(err => setError(err.message))
            .finally(() => setLoading(false));
    }, [planIndex, initData]);

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
                headers: {
                    'Authorization': `twa ${initData}`
                },
                body: formData
            });
            if (!res.ok) {
                const errText = await res.text();
                console.error('[Checkout] Upload error response:', res.status, errText);
                throw new Error(errText || `Upload failed (${res.status})`);
            }
            const data = await res.json();
            setVerificationResult(data);
        } catch (err: any) {
            console.error('[Checkout] Upload exception:', err);
            setVerificationResult({ status: 'failed', message: err?.message || 'Upload failed. Please try again.' });
        } finally {
            setUploading(false);
            // Reset file input so same file can be re-selected
            if (fileInputRef.current) fileInputRef.current.value = '';
        }
    };

    if (loading) return <div className="text-center p-8 animate-pulse">Initializing purchase...</div>;
    if (error) return (
        <div className="min-h-screen p-4 flex flex-col items-center justify-center gap-4">
            <div className="text-5xl">❌</div>
            <p className="text-red-500 text-center">{error}</p>
            <button onClick={() => navigate('/plans')} className="px-6 py-3 bg-gray-200 dark:bg-gray-700 rounded-xl font-medium">Back to Plans</button>
        </div>
    );

    if (verificationResult?.status === 'success') {
        return (
            <div className="min-h-screen p-8 flex flex-col items-center justify-center text-center gap-4">
                <div className="text-6xl">✅</div>
                <h1 className="text-2xl font-bold">Payment Verified!</h1>
                <p className="text-gray-500">Your subscription is being activated.</p>
                <button
                    onClick={() => navigate('/')}
                    className="w-full py-3 bg-[#007AFF] text-white rounded-xl font-bold mt-8"
                >
                    Go to Home
                </button>
            </div>
        );
    }

    return (
        <div className="min-h-screen p-4 flex flex-col gap-4">
            {/* Step indicator */}
            <div className="flex items-center justify-center gap-2 text-xs text-gray-400">
                <span className="text-green-500">✓ Plan</span>
                <span>→</span>
                <span className="text-[#007AFF] font-bold">Payment</span>
                <span>→</span>
                <span>Verify</span>
            </div>

            {/* Payment card */}
            <div className="bg-white dark:bg-gray-800 p-5 rounded-xl shadow-sm border border-gray-100 dark:border-gray-700">
                <h2 className="text-xl font-bold mb-4">💳 Payment Details</h2>

                <div className="space-y-3">
                    {/* Amount */}
                    <div className="bg-blue-50 dark:bg-blue-900/30 p-4 rounded-lg text-center">
                        <div className="text-xs uppercase text-gray-500 mb-1">Amount to Send</div>
                        <div className="text-3xl font-bold text-[#007AFF]">
                            {purchase?.amount?.toLocaleString()} {purchase?.currency}
                        </div>
                    </div>

                    {/* Phone */}
                    <div className="bg-gray-50 dark:bg-gray-900 p-4 rounded-lg">
                        <div className="text-xs uppercase text-gray-500 mb-1">Send to Phone Number</div>
                        <div className="text-lg font-mono font-bold">{purchase?.payment_phone}</div>
                    </div>

                    {/* Accepted methods */}
                    <div className="flex gap-2 justify-center">
                        <span className="px-3 py-1 bg-gray-100 dark:bg-gray-700 rounded-full text-sm">KPay</span>
                        <span className="px-3 py-1 bg-gray-100 dark:bg-gray-700 rounded-full text-sm">Wave</span>
                        <span className="px-3 py-1 bg-gray-100 dark:bg-gray-700 rounded-full text-sm">AYA Pay</span>
                    </div>

                    {/* Warning */}
                    <div className="bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800 p-3 rounded-lg text-sm text-center">
                        ⚠️ <strong>Do NOT</strong> add any note or remark
                    </div>
                </div>
            </div>

            <div className="flex-1"></div>

            {/* Error message */}
            {verificationResult?.status === 'failed' && (
                <div className="p-3 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 text-red-600 dark:text-red-400 rounded-lg text-sm text-center">
                    {verificationResult.message}
                </div>
            )}

            {/* Upload */}
            <input
                type="file"
                ref={fileInputRef}
                onChange={handleFileUpload}
                className="hidden"
                accept="image/*"
            />

            <button
                disabled={uploading}
                onClick={() => fileInputRef.current?.click()}
                className="w-full py-4 bg-[#007AFF] disabled:bg-gray-400 text-white rounded-xl font-bold text-lg shadow-lg active:scale-95 transition-transform flex items-center justify-center gap-2"
            >
                {uploading ? '🔍 Verifying...' : '📸 Upload Payment Screenshot'}
            </button>
        </div>
    );
}
