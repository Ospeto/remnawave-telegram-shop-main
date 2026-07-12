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
