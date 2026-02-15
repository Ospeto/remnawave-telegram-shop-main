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
            tg.BackButton.onClick(() => navigate('/plans')); // Go back to plans explicitly
        }
        return () => {
            // Cleanup handled by next page
        };
    }, [tg, navigate]);

    const purchaseCreated = useRef(false);

    useEffect(() => {
        if (!planIndex || !initData || purchaseCreated.current) return;
        purchaseCreated.current = true;

        // Create purchase immediately
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
            const data = await res.json();
            setVerificationResult(data);
        } catch (err) {
            setVerificationResult({ status: 'failed', message: 'Upload failed. Please try again.' });
        } finally {
            setUploading(false);
        }
    };

    if (loading) return <div className="text-center p-8">Initializing purchase...</div>;
    if (error) return <div className="text-center p-8 text-red-500">Error: {error}</div>;

    if (verificationResult?.status === 'success') {
        return (
            <div className="min-h-screen p-8 flex flex-col items-center justify-center text-center gap-4">
                <div className="text-6xl">✅</div>
                <h1 className="text-2xl font-bold">Success!</h1>
                <p>{verificationResult.message}</p>
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
        <div className="min-h-screen p-4 flex flex-col gap-6">
            <div className="bg-white dark:bg-gray-800 p-6 rounded-xl shadow-sm border border-gray-100 dark:border-gray-700">
                <h2 className="text-xl font-bold mb-4">Payment Instructions</h2>

                <div className="space-y-4">
                    <p className="text-gray-600 dark:text-gray-300 whitespace-pre-line text-sm">
                        {purchase?.instructions}
                    </p>

                    <div className="bg-gray-50 dark:bg-gray-900 p-4 rounded-lg text-center">
                        <div className="text-xs text-uppercase text-gray-500">Amount to Send</div>
                        <div className="text-2xl font-bold text-[#007AFF]">{purchase?.amount.toLocaleString()} {purchase?.currency}</div>
                    </div>

                    <div className="text-xs text-gray-400 text-center">
                        Accepts: KPay, Wave, AYA Pay
                    </div>
                </div>
            </div>

            <div className="flex-1"></div>

            {verificationResult?.status === 'failed' && (
                <div className="p-3 bg-red-100 text-red-600 rounded-lg text-sm text-center">
                    {verificationResult.message}
                </div>
            )}

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
                {uploading ? 'Verifying...' : '📸 Upload Payment Screenshot'}
            </button>
        </div>
    );
}
