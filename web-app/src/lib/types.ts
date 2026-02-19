export interface Plan {
    label: string;
    days: number;
    price: number;
    traffic_limit_gb: number;
    currency: string;
}

export interface SubscriptionKey {
    id: number;
    label: string;
    username: string;
    subscription_url: string;
    happ_link: string;
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
}
