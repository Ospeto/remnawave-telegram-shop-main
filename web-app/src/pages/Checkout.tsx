import { useEffect, useState, useRef, useCallback } from 'react';
import { copyText, createIdempotencyKey, openTelegramShareLink, useTelegram } from '../lib/twa';
import { useParams, useNavigate, useSearchParams } from 'react-router-dom';
import { useLanguage } from '../lib/LanguageContext';
import { LoadingScreen } from '../components/LoadingScreen';
import { ErrorScreen } from '../components/ErrorScreen';
import { SessionExpiredScreen } from '../components/SessionExpiredScreen';
import { TipBox } from '../components/TipBox';
import { useMXBrownSound } from '../lib/useMXBrownSound';
import { Plan, UserData } from '../lib/types';
import { openHappLink } from '../lib/openHapp';
import { buildTelegramStartUrl } from '../lib/externalLinks';
import { APIError, isAPIStatus } from '../lib/http';
import { clearTelegramSession, fetchJSONWithTelegramAuth, fetchUserScopedJSONWithTelegramAuth, fetchWithTelegramAuth } from '../lib/auth';
import { getVisiblePlans, resolvePlanReference } from '../lib/plans';

interface PaymentProvider {
    key: string;
    label: string;
    phone: string;
    account_name?: string;
}

interface PurchaseResponse {
    purchase_id: number;
    payment_phone: string;
    payment_phones?: Record<string, string>;
    payment_providers?: PaymentProvider[];
    amount: number;
    currency: string;
    instructions: string;
    invoice_type: string;
    extend_key_id?: number | null;
    bot_url?: string;
    happ_link?: string;
    redirect_url?: string;
}

interface PendingPurchaseErrorResponse {
    code?: string;
    message?: string;
    pending_purchase?: PurchaseResponse;
}

interface VerificationResponse {
    status: string;
    message: string;
    happ_link?: string;
    redirect_url?: string;
    test_mode?: boolean;
    shadow_passed?: boolean;
}

type CheckoutAction = 'manual' | 'wallet' | 'topup';

export function Checkout() {
    const { planIndex } = useParams();
    const { tg, initData, close } = useTelegram();
    const { t } = useLanguage();
    const { playClick } = useMXBrownSound();
    const navigate = useNavigate();
    const [searchParams] = useSearchParams();

    const extendKeyId = searchParams.get('extend');
    const promoCodeFromUrl = searchParams.get('promo');
    // URL `discount` is navigation/display metadata only; backend owns promo pricing.
    // Do not use it for wallet eligibility or button amounts before purchase creation.
    const isWalletTopup = searchParams.get('walletTopup') === 'true';
    const amountParam = searchParams.get('amount');
    const parsedTopUpAmount = Number(amountParam);
    const hasValidTopUpAmount = isWalletTopup && Number.isFinite(parsedTopUpAmount) && parsedTopUpAmount > 0;
    const backTarget = `/plans${searchParams.toString() ? `?${searchParams.toString()}` : ''}`;

    const [plans, setPlans] = useState<Plan[]>([]);
    const [userData, setUserData] = useState<UserData | null>(null);
    const [purchase, setPurchase] = useState<PurchaseResponse | null>(null);
    const [loading, setLoading] = useState(true);
    const [loadError, setLoadError] = useState<string | null>(null);
    const [purchaseError, setPurchaseError] = useState<string | null>(null);
    const [uploading, setUploading] = useState(false);
    const [verificationResult, setVerificationResult] = useState<VerificationResponse | null>(null);
    const [phoneCopied, setPhoneCopied] = useState<string | null>(null);
    const [selectedProvider, setSelectedProvider] = useState<string | null>(null);

    // Wallet payment state
    const [walletBalance, setWalletBalance] = useState<number | null>(null);
    const [walletBalanceLoading, setWalletBalanceLoading] = useState(false);
    const [walletBalanceError, setWalletBalanceError] = useState<string | null>(null);
    const [creatingPurchase, setCreatingPurchase] = useState(false);
    const [selectedAction, setSelectedAction] = useState<CheckoutAction | null>(null);
    const [walletPayError, setWalletPayError] = useState<string | null>(null);
    const [pendingPaymentNotice, setPendingPaymentNotice] = useState<string | null>(null);
    const [cancellingPayment, setCancellingPayment] = useState(false);
    const [cancelPaymentError, setCancelPaymentError] = useState<string | null>(null);
    const [authExpired, setAuthExpired] = useState(false);

    const fileInputRef = useRef<HTMLInputElement>(null);
    const purchaseIntentRef = useRef<{ action: CheckoutAction | null; key: string | null }>({ action: null, key: null });
    const resolvedPlan = resolvePlanReference(plans, planIndex);
    const selectedPlan = resolvedPlan.plan;
    const extendingKey = extendKeyId && userData?.keys
        ? userData.keys.find((key) => key.id === Number(extendKeyId))
        : undefined;
    const fallbackProviderLabels: Record<string, string> = { kpay: 'KPay', wavepay: 'WavePay', ayapay: 'AYA Pay' };
    const paymentProviders: PaymentProvider[] = purchase?.payment_providers && purchase.payment_providers.length > 0
        ? purchase.payment_providers
        : purchase?.payment_phones
            ? Object.entries(purchase.payment_phones).map(([key, phone]) => ({
                key,
                label: fallbackProviderLabels[key] || key,
                phone,
            }))
            : [];
    const paymentMethodsText = paymentProviders.map((provider) => provider.label).join(' · ');
    const checkoutReferralUrl = buildTelegramStartUrl(
        purchase?.bot_url,
        tg?.initDataUnsafe?.user?.id ? `ref_${tg.initDataUnsafe.user.id}` : '',
    );

    const setActivePurchase = useCallback((data: PurchaseResponse) => {
        setPurchase(data);
        const providerKeys = Array.isArray(data.payment_providers) && data.payment_providers.length > 0
            ? data.payment_providers.map((provider: PaymentProvider) => provider.key)
            : (data.payment_phones ? Object.keys(data.payment_phones) : []);
        setSelectedProvider(providerKeys[0] || null);
    }, []);

    const resetPendingPurchaseState = useCallback(() => {
        setPurchase(null);
        setPendingPaymentNotice(null);
        setVerificationResult(null);
        setSelectedProvider(null);
        setSelectedAction(null);
        setWalletPayError(null);
        setPurchaseError(null);
        setCancelPaymentError(null);
        purchaseIntentRef.current = { action: null, key: null };
    }, []);

    const handleBack = useCallback(() => {
        navigate(backTarget);
    }, [backTarget, navigate]);

    useEffect(() => {
        if (!tg) return;
        tg.BackButton.show();
        tg.BackButton.onClick(handleBack);
        return () => tg.BackButton.offClick(handleBack);
    }, [tg, handleBack]);

    const loadWalletBalance = useCallback(async () => {
        if (!initData || isWalletTopup) return;

        setWalletBalanceLoading(true);
        setWalletBalanceError(null);
        try {
            const data = await fetchJSONWithTelegramAuth<{ balance?: number }>('/api/wallet', initData);
            if (typeof data.balance === 'number') {
                setWalletBalance(data.balance);
            } else {
                setWalletBalance(null);
                setWalletBalanceError(t('wallet_balance_unavailable'));
            }
        } catch (err) {
            if (isAPIStatus(err, 401)) {
                clearTelegramSession();
                setAuthExpired(true);
                return;
            }
            setWalletBalance(null);
            setWalletBalanceError(t('wallet_balance_unavailable'));
        } finally {
            setWalletBalanceLoading(false);
        }
    }, [initData, isWalletTopup, t]);

    const loadCheckoutData = useCallback(async () => {
        if (!initData) {
            setLoading(false);
            return;
        }

        if (isWalletTopup && !hasValidTopUpAmount) {
            setLoadError(t('invalid_plan_selected'));
            setLoading(false);
            return;
        }

        if (!isWalletTopup && !planIndex) {
            setLoadError(t('invalid_plan_selected'));
            setLoading(false);
            return;
        }

        setLoading(true);
        setLoadError(null);
        setPurchase(null);
        setPurchaseError(null);
        setVerificationResult(null);
        setSelectedAction(null);
        setSelectedProvider(null);
        setPendingPaymentNotice(null);
        setWalletPayError(null);
        setWalletBalanceError(null);
        setWalletBalance(null);
        setCancelPaymentError(null);
        setCancellingPayment(false);
        setAuthExpired(false);
        purchaseIntentRef.current = { action: null, key: null };

        try {
            const plansData = await fetchJSONWithTelegramAuth<Plan[]>('/api/plans', initData);
            const normalizedPlans = getVisiblePlans(Array.isArray(plansData) ? plansData : []);

            if (!isWalletTopup && !resolvePlanReference(normalizedPlans, planIndex).plan) {
                setLoadError(t('invalid_plan_selected'));
                return;
            }

            setPlans(normalizedPlans);

            if (isWalletTopup) {
                setUserData(null);
                setWalletBalanceLoading(false);
                return;
            }

            const meData = await fetchUserScopedJSONWithTelegramAuth<UserData>(
                '/api/me',
                initData,
                tg?.initDataUnsafe?.user?.id,
            );
            setUserData(meData);
            await loadWalletBalance();
        } catch (err) {
            console.warn('Checkout load error:', err);
            if (isAPIStatus(err, 401)) {
                clearTelegramSession();
                setAuthExpired(true);
                return;
            }
            if (err instanceof APIError && err.body) {
                setLoadError(err.body);
                return;
            }
            setLoadError(err instanceof Error && err.message ? err.message : t('plans_load_error'));
        } finally {
            setLoading(false);
        }
    }, [hasValidTopUpAmount, initData, isWalletTopup, loadWalletBalance, planIndex, t, tg]);

    useEffect(() => {
        void loadCheckoutData();
    }, [loadCheckoutData]);

    const createPurchase = useCallback(async (action: CheckoutAction) => {
        if (!initData || creatingPurchase) return;

        setCreatingPurchase(true);
        setSelectedAction(action);
        setPurchaseError(null);
        setWalletPayError(null);
        setPendingPaymentNotice(null);
        setCancelPaymentError(null);

        try {
            const body: Record<string, unknown> = {};

            if (extendKeyId) {
                body.extend_key_id = Number(extendKeyId);
            }
            // Resellers cannot combine promo with wholesale; never send promo_code for them.
            const isResellerBuyer = !!userData?.is_reseller;
            if (promoCodeFromUrl && !isResellerBuyer) {
                body.promo_code = promoCodeFromUrl;
            }
            if (!isWalletTopup) {
                if (resolvedPlan.legacyIndex !== null) {
                    body.plan_index = resolvedPlan.legacyIndex;
                } else if (selectedPlan) {
                    body.plan_id = selectedPlan.id;
                }
            }
            if (action === 'wallet') {
                body.payment_method = 'wallet';
            }
            if (action === 'topup') {
                body.payment_method = 'wallet_topup';
                if (hasValidTopUpAmount) {
                    body.amount = parsedTopUpAmount;
                }
            }

            const res = await fetchWithTelegramAuth('/api/purchase', initData, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Idempotency-Key': purchaseIntentRef.current.action === action && purchaseIntentRef.current.key
                        ? purchaseIntentRef.current.key
                        : (() => {
                            const key = createIdempotencyKey();
                            purchaseIntentRef.current = { action, key };
                            return key;
                        })(),
                },
                body: JSON.stringify(body),
            });

            if (!res.ok) {
                if (res.status === 401) {
                    clearTelegramSession();
                    setAuthExpired(true);
                    return;
                }
                const text = await res.text();
                let pendingPurchase: PurchaseResponse | undefined;
                let message = text;
                try {
                    const parsed = JSON.parse(text) as PendingPurchaseErrorResponse;
                    if (parsed?.code === 'pending_screenshot_payment' && parsed.pending_purchase?.purchase_id) {
                        pendingPurchase = parsed.pending_purchase;
                        message = parsed.message || message;
                    } else if (parsed?.message) {
                        message = parsed.message;
                    }
                } catch {
                    // Plain-text API errors are still supported.
                }
                if (res.status === 409 && pendingPurchase) {
                    setActivePurchase(pendingPurchase);
                    const amount = (pendingPurchase.amount || 0).toLocaleString();
                    const currency = pendingPurchase.currency || selectedPlan?.currency || 'MMK';
                    setPendingPaymentNotice(t('pending_payment_resume', { amount, currency }));
                    setCancelPaymentError(null);
                    setPurchaseError(null);
                    setSelectedAction(pendingPurchase.invoice_type === 'wallet_topup' ? 'topup' : 'manual');
                    return;
                }
                throw new Error(message || t('creating_purchase'));
            }

            const data = await res.json();
            setActivePurchase(data);

            if (action === 'wallet') {
                setVerificationResult({
                    status: 'success',
                    message: t('wallet_pay_success'),
                    happ_link: data.happ_link,
                    redirect_url: data.redirect_url,
                });
            }
        } catch (err: unknown) {
            const message = err instanceof Error && err.message ? err.message : t('creating_purchase');
            if (action === 'wallet') {
                setWalletPayError(message);
            } else {
                setPurchaseError(message);
                setSelectedAction(null);
            }
        } finally {
            setCreatingPurchase(false);
        }
    }, [creatingPurchase, extendKeyId, hasValidTopUpAmount, initData, isWalletTopup, parsedTopUpAmount, promoCodeFromUrl, resolvedPlan.legacyIndex, selectedPlan, setActivePurchase, t, userData?.is_reseller]);

    const cancelPendingPayment = useCallback(async () => {
        if (!initData || !purchase || cancellingPayment) return;

        setCancellingPayment(true);
        setCancelPaymentError(null);
        try {
            const res = await fetchWithTelegramAuth(`/api/purchase/cancel?id=${purchase.purchase_id}`, initData, {
                method: 'POST',
            });
            if (!res.ok) {
                if (res.status === 401) {
                    clearTelegramSession();
                    setAuthExpired(true);
                    return;
                }
                const errText = await res.text();
                throw new Error(errText.trim() || t('pending_payment_cancel_error'));
            }

            resetPendingPurchaseState();
            navigate(backTarget);
        } catch (err: unknown) {
            const message = err instanceof Error && err.message ? err.message : t('pending_payment_cancel_error');
            setCancelPaymentError(message);
        } finally {
            setCancellingPayment(false);
        }
    }, [backTarget, cancellingPayment, initData, navigate, purchase, resetPendingPurchaseState, t]);

    const handleFileUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
        const file = e.target.files?.[0];
        if (!file || !purchase) return;

        setUploading(true);
        setVerificationResult(null);
        const formData = new FormData();
        formData.append('file', file);

        try {
            const res = await fetchWithTelegramAuth(`/api/upload_screenshot?id=${purchase.purchase_id}`, initData, {
                method: 'POST',
                body: formData
            });
            if (!res.ok) {
                if (res.status === 401) {
                    clearTelegramSession();
                    setAuthExpired(true);
                    return;
                }
                if (res.status === 429) {
                    setVerificationResult({ status: 'failed', message: t('verify_retry_wait') });
                    return;
                }
                const errText = await res.text();
                throw new Error(errText || `Upload failed (${res.status})`);
            }
            const data = await res.json();
            setVerificationResult(data as VerificationResponse);
        } catch (err: unknown) {
            const msg = err instanceof Error ? err.message : 'Upload failed. Please try again.';
            setVerificationResult({ status: 'failed', message: msg });
        } finally {
            setUploading(false);
            if (fileInputRef.current) fileInputRef.current.value = '';
        }
    };

    const copyToClipboard = (text: string) => {
        void copyText(text).catch((err) => {
            console.warn('Clipboard copy failed:', err);
        });
    };

    const handleHappLink = (happUrl: string, redirectUrl?: string) => {
        openHappLink(happUrl, redirectUrl, tg ?? null);
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

    if (loading) return <LoadingScreen message={t('loading_plans')} />;

    if (loadError) {
        return (
            <ErrorScreen
                message={loadError}
                onRetry={() => { playClick(); void loadCheckoutData(); }}
                retryLabel={t('retry')}
            />
        );
    }

    const activeCheckoutIsWalletTopup = purchase?.invoice_type === 'wallet_topup' || (isWalletTopup && purchase?.invoice_type !== 'mobile_banking');
    const activeCheckoutIsExtension = !activeCheckoutIsWalletTopup && (purchase ? Boolean(purchase.extend_key_id) : Boolean(extendKeyId));

    if (verificationResult?.status === 'success') {
        return (
            <div className="page-wrapper animate-fade-in">
                <div className="success-shell animate-slide-up">
                    <div style={{ fontSize: 64, marginBottom: 4 }} aria-hidden="true">✅</div>
                    <h1 style={{ fontSize: 22, fontWeight: 700, margin: 0 }}>{t('success_title')}</h1>
                    <p className="text-hint" style={{ margin: 0, fontSize: 14 }}>
                        {activeCheckoutIsWalletTopup ? t('success_topup_desc') : (activeCheckoutIsExtension ? t('success_extend') : t('success_new'))}
                    </p>

                    {verificationResult?.test_mode && (
                        <TipBox
                            variant={verificationResult.shadow_passed === false ? 'warning' : 'info'}
                            icon={verificationResult.shadow_passed === false ? '🧪' : '✅'}
                        >
                            {verificationResult.message}
                        </TipBox>
                    )}

                    {verificationResult?.happ_link && !activeCheckoutIsWalletTopup && (
                        <>
                            <button
                                className="btn-primary"
                                onClick={() => { playClick(); handleHappLink(verificationResult.happ_link!, verificationResult.redirect_url); }}
                                style={{ width: '100%', padding: '14px', fontSize: 15, fontWeight: 700, boxShadow: '0 4px 16px rgba(0,122,255,0.3)' }}
                            >
                                {t('btn_open_happ')}
                            </button>
                            <p className="text-hint" style={{ margin: '-4px 0 0', fontSize: 11 }}>
                                {t('success_happ_hint')}
                            </p>
                        </>
                    )}

                    {!verificationResult?.happ_link && !activeCheckoutIsWalletTopup && (
                        <TipBox variant="success" icon="✨">
                            {t('check_home_for_key')}
                        </TipBox>
                    )}

                    <TipBox variant="success" icon="💡">
                        {activeCheckoutIsWalletTopup ? t('funds_added') : (activeCheckoutIsExtension ? t('success_tip_extend') : t('success_tip_new'))}
                    </TipBox>

                    {checkoutReferralUrl && (
                        <div style={{
                            padding: '16px 20px',
                            borderRadius: 16,
                            background: 'linear-gradient(135deg, rgba(201,168,76,0.1) 0%, rgba(184,144,42,0.1) 100%)',
                            border: '1px solid rgba(201,168,76,0.25)',
                            textAlign: 'center'
                        }}>
                            <div style={{ fontSize: 16, fontWeight: 700, color: 'var(--tg-text)', marginBottom: 6 }}>
                                {t('referral_checkout_title')}
                            </div>
                            <div className="text-hint" style={{ fontSize: 13, marginBottom: 16, lineHeight: 1.4 }}>
                                {t('referral_checkout_desc')}
                            </div>
                            <button
                                className="btn-primary"
                                onClick={() => {
                                    playClick();
                                    openTelegramShareLink(tg, checkoutReferralUrl, t('referral_share_text'));
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
                    )}

                    <div className="success-shell-actions">
                        <button className="btn-secondary" onClick={() => { playClick(); navigate(activeCheckoutIsWalletTopup ? '/wallet' : '/'); }} style={{ width: '100%' }}>
                            {activeCheckoutIsWalletTopup ? t('back_to_wallet') : t('go_home')}
                        </button>
                    </div>
                </div>
            </div>
        );
    }

    // Pre-purchase wallet eligibility uses full server plan price only.
    // URL discount is not trusted; promo is applied server-side via promo_code.
    const showResellerBadge = !isWalletTopup && selectedPlan?.pricing_tier === 'wholesale';
    const topUpAmount = isWalletTopup && hasValidTopUpAmount ? parsedTopUpAmount : 0;
    const targetAmount = isWalletTopup ? topUpAmount : (selectedPlan?.price ?? 0);
    const canPayWithWallet = !isWalletTopup && selectedPlan !== undefined && walletBalance !== null && walletBalance >= targetAmount;
    const canShowWalletOption = !isWalletTopup && selectedPlan !== undefined;
    const isManualPurchaseReady = !!purchase && purchase.invoice_type !== 'wallet_payment';
    const displayAmount = pendingPaymentNotice && purchase ? purchase.amount : targetAmount;
    const displayCurrency = pendingPaymentNotice && purchase ? (purchase.currency || selectedPlan?.currency || 'MMK') : (selectedPlan?.currency || 'MMK');
    const displayLabel = pendingPaymentNotice && purchase
        ? t('pending_payment_current_label')
        : (isWalletTopup ? t('top_up_amount', { amount: targetAmount.toLocaleString(), currency: selectedPlan?.currency || 'MMK' }) : selectedPlan?.label);

    return (
        <div className="page-wrapper animate-fade-in" style={{ gap: 16 }}>
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
            <div style={{ textAlign: 'center', fontSize: 16, fontWeight: 700 }}>
                {isWalletTopup ? t('title_top_up') : (extendKeyId ? t('title_extend') : t('title_choose_plan'))}
            </div>

            <div className="glass-card" style={{ padding: 18 }}>
                <h2 style={{ fontSize: 17, fontWeight: 700, margin: '0 0 12px' }}>
                    {t('choose_payment_method')}
                </h2>

                <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, alignItems: 'center', marginBottom: 12 }}>
                    <div style={{ display: 'flex', flexDirection: 'column', gap: 4 }}>
                        <span className="text-hint">{displayLabel}</span>
                        {showResellerBadge && (
                            <span
                                style={{
                                    display: 'inline-block',
                                    alignSelf: 'flex-start',
                                    background: 'rgba(0, 122, 255, 0.12)',
                                    color: 'var(--tg-link, #007AFF)',
                                    border: '1px solid rgba(0, 122, 255, 0.22)',
                                    borderRadius: 999,
                                    padding: '2px 8px',
                                    fontSize: 10,
                                    fontWeight: 700,
                                    letterSpacing: 0.2,
                                }}
                            >
                                {t('reseller_price_badge')}
                            </span>
                        )}
                    </div>
                    <strong>{displayAmount.toLocaleString()} {displayCurrency}</strong>
                </div>

                {extendKeyId && extendingKey && (
                    <TipBox variant="info" icon="ℹ️" style={{ marginBottom: 12 }}>
                        {t('subtitle_extending', { label: extendingKey.label })}{extendingKey.expire_at ? ` · ${new Date(extendingKey.expire_at).toLocaleDateString()}` : ''}
                    </TipBox>
                )}

                {purchaseError && (
                    <div role="alert" style={{
                        padding: 10, borderRadius: 8, marginBottom: 10,
                        background: 'rgba(255, 59, 48, 0.08)', border: '1px solid rgba(255, 59, 48, 0.15)',
                        color: 'var(--color-danger)', fontSize: 13
                    }}>
                        {purchaseError}
                    </div>
                )}

                {pendingPaymentNotice && purchase && (
                    <TipBox variant="warning" icon="!" style={{ marginBottom: 12 }}>
                        {pendingPaymentNotice}
                    </TipBox>
                )}

                {pendingPaymentNotice && purchase ? (
                    <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
                        <div className="text-hint" style={{ fontSize: 12 }}>
                            {t('pending_payment_upload_hint')}
                        </div>
                        {cancelPaymentError && (
                            <div role="alert" style={{
                                padding: 10, borderRadius: 8,
                                background: 'rgba(255, 59, 48, 0.08)', border: '1px solid rgba(255, 59, 48, 0.15)',
                                color: 'var(--color-danger)', fontSize: 13
                            }}>
                                {cancelPaymentError}
                            </div>
                        )}
                        <button
                            className="btn-secondary"
                            onClick={() => { playClick(); void cancelPendingPayment(); }}
                            disabled={cancellingPayment || uploading}
                            style={{ width: '100%', opacity: cancellingPayment ? 0.7 : 1 }}
                        >
                            {cancellingPayment
                                ? <><div className="spinner" style={{ width: 16, height: 16, borderWidth: 2 }} />{t('pending_payment_canceling')}</>
                                : t('pending_payment_cancel_btn')}
                        </button>
                    </div>
                ) : isWalletTopup ? (
                    <button
                        className="btn-primary"
                        onClick={() => { playClick(); void createPurchase('topup'); }}
                        disabled={creatingPurchase}
                        style={{ width: '100%', background: 'var(--color-success)', opacity: creatingPurchase && selectedAction === 'topup' ? 0.7 : 1 }}
                    >
                        {creatingPurchase && selectedAction === 'topup'
                            ? <><div className="spinner" style={{ width: 16, height: 16, borderWidth: 2 }} />{t('creating_purchase')}</>
                            : t('create_payment_request')}
                    </button>
                ) : (
                    <div style={{ display: 'flex', flexDirection: 'column', gap: 10 }}>
                        {canShowWalletOption && (
                            <button
                                className="btn-primary"
                                onClick={() => { playClick(); void createPurchase('wallet'); }}
                                disabled={creatingPurchase || !canPayWithWallet || walletBalanceLoading || walletBalanceError !== null}
                                style={{ width: '100%', background: 'var(--color-success)', opacity: creatingPurchase && selectedAction === 'wallet' ? 0.7 : (!canPayWithWallet ? 0.5 : 1) }}
                            >
                                {creatingPurchase && selectedAction === 'wallet'
                                    ? <><div className="spinner" style={{ width: 16, height: 16, borderWidth: 2 }} />{t('wallet_pay_processing')}</>
                                    : t('wallet_pay_btn', { amount: targetAmount.toLocaleString(), currency: selectedPlan?.currency || 'MMK' })}
                            </button>
                        )}

                        {canShowWalletOption && walletBalanceLoading && (
                            <div className="text-hint" style={{ fontSize: 12 }}>
                                {t('wallet_balance_loading')}
                            </div>
                        )}

                        {canShowWalletOption && walletBalanceError && (
                            <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                                <div className="text-hint" style={{ fontSize: 12 }}>
                                    {walletBalanceError}
                                </div>
                                <button
                                    className="btn-secondary"
                                    onClick={() => { playClick(); void loadWalletBalance(); }}
                                    style={{ width: '100%' }}
                                >
                                    {t('wallet_balance_retry')}
                                </button>
                            </div>
                        )}

                        {canShowWalletOption && !walletBalanceLoading && !walletBalanceError && walletBalance === null && (
                            <div className="text-hint" style={{ fontSize: 12 }}>
                                {t('wallet_balance_unavailable')}
                            </div>
                        )}

                        {canShowWalletOption && !walletBalanceLoading && !walletBalanceError && walletBalance !== null && !canPayWithWallet && (
                            <div className="text-hint" style={{ fontSize: 12 }}>
                                {t('wallet_balance_low')}
                            </div>
                        )}

                        <button
                            className="btn-secondary"
                            onClick={() => { playClick(); void createPurchase('manual'); }}
                            disabled={creatingPurchase}
                            style={{ width: '100%', opacity: creatingPurchase && selectedAction === 'manual' ? 0.7 : 1 }}
                        >
                            {creatingPurchase && selectedAction === 'manual'
                                ? <><div className="spinner" style={{ width: 16, height: 16, borderWidth: 2 }} />{t('creating_purchase')}</>
                                : t('pay_with_mobile_banking')}
                        </button>
                    </div>
                )}
            </div>

            {isManualPurchaseReady && purchase && (
                <div className="glass-card" style={{ padding: 20 }}>
                    <h2 style={{ fontSize: 17, fontWeight: 700, margin: '0 0 20px' }}>
                        {t('guide_title')}
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
                            <div className="text-hint" style={{ fontSize: 12, marginTop: 2 }}>
                                {t('guide_step_1_hint', { methods: paymentMethodsText || t('choose_payment_method') })}
                            </div>
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
                                <div style={{ display: 'flex', alignItems: 'baseline', gap: 6 }}>
                                    <span className="text-hint" style={{ fontSize: 12 }}>{t('label_amount')}:</span>
                                    <span style={{ fontWeight: 800, fontSize: 22, letterSpacing: '-0.5px' }}>{(purchase?.amount || 0).toLocaleString()}</span>
                                    <span className="text-hint" style={{ fontSize: 13 }}>{purchase?.currency}</span>
                                </div>
                                {paymentProviders.length > 0 ? (() => {
                                    const effectiveSelected = selectedProvider ?? paymentProviders[0].key;
                                    const selectedProviderConfig = paymentProviders.find((provider) => provider.key === effectiveSelected) || paymentProviders[0];

                                    return (
                                        <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
                                            {paymentProviders.length > 1 && (
                                                <div style={{ display: 'flex', gap: 6 }}>
                                                    {paymentProviders.map((provider) => (
                                                        <button
                                                            key={provider.key}
                                                            onClick={() => { playClick(); setSelectedProvider(provider.key); }}
                                                            style={{
                                                                flex: 1,
                                                                padding: '8px 4px',
                                                                borderRadius: 10,
                                                                border: effectiveSelected === provider.key ? '2px solid var(--accent-color)' : '1px solid var(--input-border)',
                                                                background: effectiveSelected === provider.key ? 'var(--accent-color)' : 'var(--input-bg)',
                                                                color: effectiveSelected === provider.key ? '#fff' : 'inherit',
                                                                fontWeight: 600,
                                                                fontSize: 13,
                                                                cursor: 'pointer',
                                                                transition: 'all 0.15s ease'
                                                            }}
                                                        >
                                                            {provider.label}
                                                        </button>
                                                    ))}
                                                </div>
                                            )}
                                            {selectedProviderConfig?.phone ? (
                                                <div style={{ padding: '10px 12px', borderRadius: 12, background: 'var(--input-bg)', border: '1px solid var(--input-border)' }}>
                                                    <div className="text-hint" style={{ fontSize: 11, marginBottom: 4 }}>{selectedProviderConfig.label}</div>
                                                    <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 4 }}>
                                                        <div style={{ fontWeight: 700, fontSize: 18, fontFamily: 'monospace', letterSpacing: '0.5px' }}>{selectedProviderConfig.phone}</div>
                                                        <button
                                                            onClick={() => {
                                                                playClick();
                                                                copyToClipboard(selectedProviderConfig.phone);
                                                                setPhoneCopied(effectiveSelected);
                                                                setTimeout(() => setPhoneCopied(null), 1500);
                                                            }}
                                                            className="btn-secondary"
                                                            aria-label={t('tap_to_copy')}
                                                            style={{ padding: '4px 8px', fontSize: 13, minWidth: 30, borderRadius: 8, color: phoneCopied === effectiveSelected ? 'var(--color-success)' : undefined }}
                                                        >
                                                            {phoneCopied === effectiveSelected ? '✓' : '📋'}
                                                        </button>
                                                    </div>
                                                    {selectedProviderConfig.account_name && (
                                                        <div style={{ marginTop: 10, display: 'flex', flexDirection: 'column', gap: 2 }}>
                                                            <span className="text-hint" style={{ fontSize: 11 }}>{t('label_account_name')}</span>
                                                            <span style={{ fontWeight: 600, fontSize: 14 }}>{selectedProviderConfig.account_name}</span>
                                                        </div>
                                                    )}
                                                </div>
                                            ) : paymentProviders.length > 1 ? (
                                                <div className="text-hint" style={{ textAlign: 'center', padding: 8, fontSize: 13 }}>
                                                    {t('select_payment_method') || 'Select a payment method above'}
                                                </div>
                                            ) : null}
                                        </div>
                                    );
                                })() : (
                                    <div style={{ padding: '10px 12px', borderRadius: 12, background: 'var(--input-bg)', border: '1px solid var(--input-border)' }}>
                                        <div className="text-hint" style={{ fontSize: 11, marginBottom: 4 }}>{t('label_phone')}</div>
                                        <div style={{ display: 'flex', alignItems: 'center', justifyContent: 'space-between', gap: 4 }}>
                                            <div style={{ fontWeight: 700, fontSize: 18, fontFamily: 'monospace', letterSpacing: '0.5px' }}>{purchase?.payment_phone}</div>
                                            <button
                                                onClick={() => {
                                                    playClick();
                                                    copyToClipboard(purchase?.payment_phone || '');
                                                    setPhoneCopied('default');
                                                    setTimeout(() => setPhoneCopied(null), 1500);
                                                }}
                                                className="btn-secondary"
                                                aria-label={t('tap_to_copy')}
                                                style={{ padding: '4px 8px', fontSize: 13, minWidth: 30, borderRadius: 8, color: phoneCopied === 'default' ? 'var(--color-success)' : undefined }}
                                            >
                                                {phoneCopied === 'default' ? '✓' : '📋'}
                                            </button>
                                        </div>
                                    </div>
                                )}
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
            )}

            <div style={{ flex: 1 }} />

            {walletPayError && (
                <div role="alert" style={{
                    padding: 12, borderRadius: 12, fontSize: 13, textAlign: 'center',
                    background: 'rgba(255, 59, 48, 0.08)', border: '1px solid rgba(255, 59, 48, 0.18)',
                    color: 'var(--color-danger)'
                }}>
                    {walletPayError}
                </div>
            )}

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
            {isManualPurchaseReady && (
                <>
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
                </>
            )}
        </div>
    );
}
