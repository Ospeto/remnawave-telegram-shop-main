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
import { APIError, fetchJSON, isAPIStatus } from '../lib/http';
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
    bot_url: string;
    happ_link?: string;
    redirect_url?: string;
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
    const promoCode = searchParams.get('promo');
    const promoDiscount = Number(searchParams.get('discount'));
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
        setWalletPayError(null);
        setWalletBalanceError(null);
        setWalletBalance(null);
        setAuthExpired(false);
        purchaseIntentRef.current = { action: null, key: null };

        try {
            const plansData = await fetchJSON<Plan[]>('/api/plans');
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

        try {
            const body: Record<string, unknown> = {};

            if (extendKeyId) {
                body.extend_key_id = Number(extendKeyId);
            }
            if (promoCode) {
                body.promo_code = promoCode;
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
                throw new Error(text || t('creating_purchase'));
            }

            const data = await res.json();
            setPurchase(data);
            const providerKeys = Array.isArray(data.payment_providers) && data.payment_providers.length > 0
                ? data.payment_providers.map((provider: PaymentProvider) => provider.key)
                : (data.payment_phones ? Object.keys(data.payment_phones) : []);
            setSelectedProvider(providerKeys[0] || null);

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
    }, [creatingPurchase, extendKeyId, hasValidTopUpAmount, initData, isWalletTopup, parsedTopUpAmount, promoCode, resolvedPlan.legacyIndex, selectedPlan, t]);

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

    if (verificationResult?.status === 'success') {
        return (
            <div className="animate-slide-up screen-center">
                <div style={{ fontSize: 64, marginBottom: 8 }} aria-hidden="true">✅</div>
                <h1 style={{ fontSize: 22, fontWeight: 700, margin: 0 }}>{t('success_title')}</h1>
                <p className="text-hint" style={{ margin: 0, fontSize: 14 }}>
                    {isWalletTopup ? t('success_topup_desc') : (extendKeyId ? t('success_extend') : t('success_new'))}
                </p>

                {verificationResult?.test_mode && (
                    <TipBox
                        variant={verificationResult.shadow_passed === false ? 'warning' : 'info'}
                        icon={verificationResult.shadow_passed === false ? '🧪' : '✅'}
                    >
                        {verificationResult.message}
                    </TipBox>
                )}

                {verificationResult?.happ_link && !isWalletTopup && (
                    <button
                        className="btn-primary"
                        onClick={() => { playClick(); handleHappLink(verificationResult.happ_link!, verificationResult.redirect_url); }}
                        style={{ marginTop: 12, width: '100%', padding: '14px', fontSize: 15, fontWeight: 700, boxShadow: '0 4px 16px rgba(0,122,255,0.3)' }}
                    >
                        {t('btn_open_happ')}
                    </button>
                )}

                {verificationResult?.happ_link && !isWalletTopup && (
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
                            const text = t('referral_share_text');
                            const url = `${botUrlToUse}?start=ref_${uid}`;
                            openTelegramShareLink(tg, url, text);
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

    const effectiveDiscountPercent = promoCode && Number.isFinite(promoDiscount) && promoDiscount > 0 ? promoDiscount : 0;
    const topUpAmount = isWalletTopup && hasValidTopUpAmount ? parsedTopUpAmount : 0;
    const baseTargetAmount = isWalletTopup ? topUpAmount : (selectedPlan?.price ?? 0);
    const targetAmount = !isWalletTopup && effectiveDiscountPercent > 0
        ? Math.round(baseTargetAmount * (1 - effectiveDiscountPercent / 100))
        : baseTargetAmount;
    const canPayWithWallet = !isWalletTopup && selectedPlan !== undefined && walletBalance !== null && walletBalance >= targetAmount;
    const canShowWalletOption = !isWalletTopup && selectedPlan !== undefined;
    const isManualPurchaseReady = !!purchase && purchase.invoice_type !== 'wallet_payment';

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
            <div style={{ textAlign: 'center', fontSize: 16, fontWeight: 700 }}>
                {isWalletTopup ? t('title_top_up') : (extendKeyId ? t('title_extend') : t('title_choose_plan'))}
            </div>

            <div className="glass-card" style={{ padding: 18 }}>
                <h2 style={{ fontSize: 17, fontWeight: 700, margin: '0 0 12px' }}>
                    {t('choose_payment_method')}
                </h2>

                <div style={{ display: 'flex', justifyContent: 'space-between', gap: 12, alignItems: 'center', marginBottom: 12 }}>
                    <span className="text-hint">{isWalletTopup ? t('top_up_amount', { amount: targetAmount.toLocaleString(), currency: selectedPlan?.currency || 'MMK' }) : selectedPlan?.label}</span>
                    <strong>{targetAmount.toLocaleString()} {selectedPlan?.currency || 'MMK'}</strong>
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

                {isWalletTopup ? (
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
