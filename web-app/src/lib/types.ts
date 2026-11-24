export interface Plan {
    id: string;
    label: string;
    days: number;
    price: number; // effective from API
    traffic_limit_gb: number;
    currency: string;
    active?: boolean;
    sort_order?: number;
    pricing_tier?: 'retail' | 'wholesale';
}

export interface AdminPlan extends Plan {
    active: boolean;
    sort_order: number;
    wholesale_price?: number | null;
}

export interface SubscriptionKey {
    id: number;
    label: string;
    username: string;
    subscription_url: string;
    happ_link: string;
    redirect_url?: string;
    expire_at: string | null;
    days_remaining: number;
    total_days: number;
    status: string;
    traffic_used_gb: number;
    traffic_limit_gb: number;
    auto_renew: boolean;
}

/**
 * An order the customer still has to act on.
 *
 * U3: `/api/me` reports it so Home and Wallet can show a banner instead of
 * leaving an unfinished payment discoverable only by hitting the 409 in
 * Checkout.
 */
export interface PendingPurchase {
    id: number;
    status: string;
    amount: number;
    currency: string;
    invoice_type: string;
    created_at: string;
    awaiting_screenshot: boolean;
    plan_label?: string;
    days?: number;
    traffic_limit_gb?: number;
    extend_key_id?: number | null;
}

export interface UserData {
    user: {
        id: number;
        telegram_id: number;
    };
    keys: SubscriptionKey[];
    is_active: boolean;
    expire_at: string | null;
    days_remaining: number;
    trial_eligible: boolean;
    trial_days: number;
    referral_count?: number;
    referral_earned?: number;
    referral_stats_unavailable?: boolean;
    referral_bonus_amount?: number;
    bot_url?: string;
    support_url?: string;
    is_admin?: boolean;
    is_reseller?: boolean;
    pending_purchase?: PendingPurchase | null;
    /**
     * True when the traffic snapshot could not be fetched before this response
     * was built, so the key figures may still read as zero.
     *
     * U4: the client re-fetches once instead of showing "0 GB used" forever.
     */
    stats_pending?: boolean;
}

export interface ResellerAccount {
    credit_limit: number;
    balance_owed: number;
    remaining_credit: number;
    is_reseller?: boolean;
}

/** Admin list / credit / toggle response for a reseller customer. */
export interface AdminReseller {
    telegram_id: number;
    is_reseller: boolean;
    credit_limit: number;
    balance_owed: number;
    remaining_credit: number;
}

export interface ResellerLedgerEntry {
    id: number;
    entry_type: string;
    direction: string;
    amount: number;
    purchase_id?: number | null;
    effective_at: string;
    note?: string | null;
    created_by: string;
}

export interface ResellerSettlementResponse {
    created: boolean;
    amount: number;
    balance_owed: number;
    remaining_credit: number;
    idempotency_key: string;
    ledger_entry_id: number;
}

export interface AdminPromo {
    code: string;
    discount_percent: number;
    max_uses: number;
    used_count: number;
    valid_until: string;
    created_at: string;
    status?: 'active' | 'expired' | 'exhausted';
}
