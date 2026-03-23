export type Language = 'en' | 'my';

export const translations: Record<Language, Record<string, string>> = {
    en: {
        // ── General ──
        'loading': 'Loading...',
        'retry': 'Try Again',
        'error_prefix': 'Error: ',
        'powered_by': 'Wavy Private Server',
        'open_in_tg': 'Open inside Telegram to use this app',
        'buy_button': 'Buy New Key',

        // ── Navigation ──
        'nav_plan': 'Plan',
        'nav_payment': 'Payment',
        'nav_verify': 'Verify',
        'back_button': 'Back',
        'back_to_plans': 'Back to Plans',
        'go_home': 'My Keys',
        'btn_open_happ': 'Open in Happ Proxy',
        'success_happ_hint': 'Tap to instantly import your key into Happ Proxy — no copy-paste needed',

        // ── Home ──
        'home_title': 'Wavy Private Server',
        'active_key_count': '{{count}} active key',
        'active_key_count_plural': '{{count}} active keys',
        'no_active_keys': 'No active keys',
        'key_active': '● Active',
        'key_expired': '○ Expired',
        'days_left': 'days left',
        'expires_on': 'Expires {{date}}',
        'data_usage': 'Data Used',
        'btn_add_happ': 'Add to Happ',
        'btn_extend': 'Extend',
        'btn_copy_key': 'Copy Key',
        'btn_buy_new': 'Buy New Key',
        'btn_get_started': 'Get Started',

        // ── Home Help / Tips ──
        'help_expired': '⏳ This key has expired. Tap Extend to add more days and get back online instantly.',
        'help_btn_add': '📱 Add to Happ — one tap to import your key into the Happ Proxy app automatically',
        'help_btn_extend': '📅 Extend — add more days and data to this key without losing your current balance',
        'help_btn_copy': '🔗 Copy Key — use this link in any VPN app that supports Outline/Shadowsocks',
        'tip_multi_key': '💡 Tip: You can have multiple keys for different devices, or extend an existing one.',
        'info_device_limit': 'Devices per Key',
        'info_device_count': '3',
        'info_servers': 'Server Locations',
        'info_server_list': 'TH · DE · US · SG · JP',
        'contact_support': 'Contact Support',

        // ── Trial ──
        'trial_button': '🎁 Start Free Trial — {{days}} Days',
        'trial_activating': 'Activating your free trial...',
        'trial_error': 'Could not activate trial. Please try again or contact support.',
        'trial_already_used': 'Trial already used',
        'trial_success': '🎉 Trial activated! Your free key is ready to use.',

        // ── Welcome (no keys yet) ──
        'welcome_title': 'Welcome to Wavy!', // Deprecated but kept safe
        'home_empty_title': 'Ready to Connect?',
        'home_empty_desc': 'You don\'t have any VPN keys yet. Get your private key and start browsing freely.',
        'home_empty_action': '🚀 Get My Key',
        'welcome_text': 'You don\'t have any VPN keys yet. Pick a plan below to get started — setup only takes a minute.',
        'download_title': '📥 Download Happ Proxy App Before Buying',
        'download_text': 'Before activating your key, download the Happ Proxy app. It\'s free and takes under 30 seconds to set up.',
        'btn_download': 'Download App',
        'how_it_works': 'How it works',
        'step_1': 'Choose a plan below',
        'step_2': 'Pay via mobile banking',
        'step_3': 'Upload a screenshot — verified in seconds',
        'step_4': 'Your VPN key is ready!',

        // ── Plans ──
        'loading_plans': 'Fetching plans...',
        'loading_wallet': 'Loading wallet...',
        'title_extend': 'Extend Your Key',
        'title_choose_plan': 'Choose a Plan',
        'title_top_up': 'Top Up Wallet',
        'choose_payment_method': 'Choose a payment method',
        'create_payment_request': 'Create payment request',
        'pay_with_mobile_banking': 'Pay via mobile banking',
        'wallet_balance_unavailable': 'Wallet balance unavailable',
        'wallet_balance_low': 'Wallet balance is not enough for wallet payment',
        'top_up_amount': '{{amount}} {{currency}}',
        'subtitle_extending': 'Extending: {{label}}',
        'subtitle_new_key': 'Choose a plan — tap to continue',
        'subtitle_new_key_hint': 'A new VPN key will be created for you',
        'best_value': '✦ Best Value',
        'plans_load_error': 'Could not load plans. Please try again.',
        'invalid_plan_selected': 'Invalid plan selected',
        'unlimited': 'Unlimited Data',
        'per_day': '{{currency}} MMK/day',
        'new_expiry': '✓ New expiry: {{date}}',

        // ── Plans Help ──
        'help_extend_info': '✅ Extending adds more days and data on top of what you have left — nothing is lost. Your current key and settings stay the same.',
        'help_new_key_info': 'Each plan gives you a dedicated VPN key. Unlimited plans have no data cap. Limited plans cost less but have a monthly data limit — great for light use.',
        'help_filtered_plans': 'Showing {{type}} plans only — matches your current {{type_desc}} key type.',
        'help_payments': '💳 Available mobile banking accounts are shown at checkout. Transfer the exact amount, then upload a screenshot for instant automated verification.',

        // ── Checkout ──
        'creating_purchase': 'Preparing your order...',
        'payment_details': 'Payment Details',
        'amount_to_send': 'Exact Amount to Transfer',
        'send_to_phone': 'Transfer to This Number',
        'warning_no_note': '⚠️ In the note/remark field, write only "Payment" — nothing else',
        'upload_btn': '📤 Upload Payment Screenshot',
        'uploading_btn': 'Verifying your payment...',
        'upload_hint': 'Send the transfer, then take a clear screenshot of the confirmation screen and upload it here. Our system verifies it automatically in seconds.',

        // ── Checkout Guide Steps ──
        'guide_title': 'How to pay',
        'guide_step_1': 'Open your banking app',
        'guide_step_1_hint': 'Available now: {{methods}}',

        'guide_step_2': 'Transfer exact amount to the selected account',
        'label_amount': 'Amount (MMK)',
        'label_phone': 'Phone',
        'label_account_name': 'Account Name',

        'guide_step_3': 'In the note/remark, write only "Payment"',
        'guide_step_3_hint': 'Do NOT write VPN, Wavy, or anything else',

        'guide_step_4': 'Screenshot the confirmation & upload below',
        'guide_step_4_hint': 'Verified automatically in seconds',

        'tap_to_copy': 'Tap to copy',
        'copied': 'Copied ✓',
        'important_warning': '⚠️ Critical: Transfer the exact amount shown. In the remark/note field, write only "Payment". Writing anything else (VPN, Wavy, Outline) will cause verification to fail and your key will not be issued.',

        // ── Success / Error ──
        'success_title': '✅ Payment Verified!',
        'success_extend': 'Your key has been extended. Extra days and data have been added — you\'re all set.',
        'success_new': 'Your new VPN key is live and ready to use. Tap below to add it to Happ.',
        'success_tip_extend': 'Go back to check your updated expiry date and remaining data.',
        'success_tip_new': 'Tap "Open in Happ Proxy" above to start using your VPN instantly, or copy the key link for any other VPN app.',
        'verify_error_tip': '💡 Common fixes: make sure the amount and receiver phone/account name match, and that the screenshot clearly shows the transaction confirmation (not just a balance screen).',

        // ── Wallet ──
        'wallet_title': 'Wavy Wallet',
        'wallet_subtitle': 'Balance & auto-renewal',
        'current_balance': 'Available Balance',
        'top_up_wallet': '+ Top Up Balance',
        'wallet_error': 'Could not load wallet details. Please try again.',
        'auto_renew_title': 'Auto-Renewal',
        'auto_renew_enabled': 'On — renews automatically when your key expires',
        'auto_renew_disabled': 'Off — tap to enable for hands-free renewal',
        'transaction_history': 'Transaction History',
        'no_transactions': 'No transactions yet',
        'transaction_topup': 'Wallet Top-up',
        'transaction_purchase': 'Plan Purchase',
        'transaction_refund': 'Refund',
        'wallet_info': 'Your wallet balance never expires and can be used anytime for any plan.',
        'no_refund_title': 'Refund Policy',
        'no_refund_desc': 'All plan purchases are final and non-refundable. Your wallet balance never expires.',

        // ── Referral ──
        'referral_earnings': '🤝 Referral Earnings',
        'friends_invited': 'Friends invited:',
        'total_earned': 'Total earned:',
        'share_link': 'Share your link →',
        'referral_pending': 'Pending',
        'referral_bonus_received': 'Bonus Received',
        'referral_share_text': 'Get 1,000 MMK free VPN balance when you buy Wavy VPN from my link! 🎁',
        'referral_wallet_chip': '{{count}} Referrals · {{earned}} Earned',
        'referral_checkout_title': '🎁 Give 1000, Get 1000',
        'referral_checkout_desc': 'Invite friends to Wavy and you both get free VPN balance!',
        'referral_checkout_btn': 'Share link',

        // ── Wallet Tips ──
        'wallet_tips_title': 'Why use Wallet?',
        'wallet_tip_1_title': '🔒 Stay Connected 24/7',
        'wallet_tip_1_desc': 'Enable auto-renew and your VPN will renew itself automatically — no interruptions, ever.',
        'wallet_tip_2_title': '⚡ Instant Key Activation',
        'wallet_tip_2_desc': 'No screenshot needed. Wallet payments are verified instantly — your key is ready in seconds.',
        'wallet_tip_3_title': '🕐 Set It and Forget It',
        'wallet_tip_3_desc': 'Top up once and pay for months. No need to open your banking app every time you renew.',

        // ── Wallet Payment ──
        'pay_with_wallet': '⚡ Pay with Wallet',
        'your_balance': 'Your balance:',
        'or_pay_manually': 'Or pay via mobile banking:',
        'accepted_methods': '{{methods}}',
        'wallet_pay_success': 'Paid with Wallet',
        'check_home_for_key': 'Your key is active — go to My Keys to use it.',
        'funds_added': 'Funds added to your wallet successfully.',
        'wallet_pay_processing': 'Processing...',
        'wallet_pay_btn': 'Pay {{amount}} {{currency}} from Wallet',
        'wallet_pay_error': 'Wallet payment failed. Please check your balance and try again.',

        // ── Wallet Top-up Success ──
        'success_topup_desc': 'Funds have been added to your wallet balance.',
        'back_to_wallet': 'Back to Wallet',

        // ── Wallet Empty States ──
        'wallet_empty_title': 'No Transactions Yet',
        'wallet_empty_desc': 'Your transaction history will appear here after your first top-up or purchase.',

        // ── Promo Code ──
        'promo_placeholder': 'Enter promo code',
        'promo_apply': 'Apply',
        'promo_validating': 'Checking...',
        'promo_valid': '🎉 Code applied — {{percent}}% off your order!',
        'promo_invalid': '❌ Code not found or has expired',
    },

    my: {
        // ── General ──
        'loading': 'ခေတ္တစောင့်ဆိုင်းပါ...',
        'retry': 'ထပ်မံကြိုးစားမည်',
        'error_prefix': 'ချို့ယွင်းချက်: ',
        'powered_by': 'Wavy Private Server',
        'open_in_tg': 'ဤ App အား Telegram အတွင်းမှသာ အသုံးပြုနိုင်ပါသည်',
        'buy_button': 'Key အသစ်ဝယ်ရန်',

        // ── Navigation ──
        'nav_plan': 'ပက်ကေ့ချ် (Plan)',
        'nav_payment': 'ငွေပေးချေမှု',
        'nav_verify': 'အတည်ပြုချက်',
        'back_button': 'နောက်သို့',
        'back_to_plans': 'ပက်ကေ့ချ် (Plan) များသို့ ပြန်သွားမည်',
        'go_home': 'ကျွန်ုပ်၏ Key များ',
        'btn_open_happ': 'Happ Proxy သို့ ဝင်ရောက်ရန်',
        'success_happ_hint': 'Happ Proxy App အတွင်းသို့ Key အား ချက်ချင်း ထည့်သွင်းရန် နှိပ်ပါ',

        // ── Home ──
        'home_title': 'Wavy Private Server',
        'active_key_count': 'လက်ရှိ အသုံးပြုနေသော Key ၁ ခု',
        'active_key_count_plural': 'လက်ရှိ အသုံးပြုနေသော Key {{count}} ခု',
        'no_active_keys': 'အသုံးပြုဆဲ Key မရှိသေးပါ',
        'key_active': '● သုံးစွဲနိုင်သည်',
        'key_expired': '○ သက်တမ်းကုန်ဆုံးသည်',
        'days_left': 'ရက်ကျန်',
        'expires_on': 'သက်တမ်းကုန်ဆုံးမည့် ရက်: {{date}}',
        'data_usage': 'Data သုံးစွဲမှုပမာဏ',
        'btn_add_happ': 'Happ သို့ ထည့်သွင်းရန်',
        'btn_extend': 'သက်တမ်းတိုးရန်',
        'btn_copy_key': 'Key ကူးယူရန်',
        'btn_buy_new': 'Key အသစ် ဝယ်ယူရန်',
        'btn_get_started': 'စတင်ရန်',

        // ── Home Help / Tips ──
        'help_expired': '⏳ ဤ Key သည် သက်တမ်းကုန်ဆုံးသွားပါပြီ။ "သက်တမ်းတိုးရန်" ကို နှိပ်ပြီး ချက်ချင်း ပြန်လည်အသက်သွင်းနိုင်ပါသည်။',
        'help_btn_add': 'Happ သို့ ထည့်သွင်းရန် — Happ Proxy App အတွင်းသို့ Key အား တစ်ချက်နှိပ်ရုံဖြင့် လွယ်ကူစွာ ထည့်သွင်းနိုင်ပါသည်',
        'help_btn_extend': 'သက်တမ်းတိုးရန် — လက်ရှိ ကျန်ရှိနေသော ရက်နှင့် Data ပေါ်တွင် ထပ်မံပေါင်းထည့်ပေးပါမည်',
        'help_btn_copy': 'Key ကူးယူရန် — Outline သို့မဟုတ် Shadowsocks ကို ထောက်ပံ့သော မည်သည့် VPN App တွင်မဆို အသုံးပြုနိုင်ပါသည်',
        'tip_multi_key': 'အကြံပြုချက်: Device ၃ ခုထက်ပို၍ အသုံးပြုလိုပါက Key အသစ်ထပ်မံဝယ်ယူနိုင်သလို၊ ရှိပြီးသား Key ကိုလည်း သက်တမ်းတိုးနိုင်ပါသည်။',
        'info_device_limit': 'Key တစ်ခုလျှင် အသုံးပြုနိုင်သော Device',
        'info_device_count': '၃ ခု',
        'info_servers': 'ဆာဗာ တည်နေရာများ',
        'info_server_list': 'TH · DE · US · SG · JP',
        'contact_support': 'Customer Support သို့ ဆက်သွယ်ရန်',

        // ── Trial ──
        'trial_button': '🎁 အခမဲ့ {{days}} ရက် စမ်းသပ်အသုံးပြုခွင့် ရယူရန်',
        'trial_activating': 'စမ်းသပ်အသုံးပြုခွင့်ကို စတင်နေပါသည်...',
        'trial_error': 'စမ်းသပ်အသုံးပြုခွင့် ရယူ၍မရပါ။ ကျေးဇူးပြု၍ ထပ်မံကြိုးစားပါ သို့မဟုတ် Customer Support သို့ ဆက်သွယ်ပါ။',
        'trial_already_used': 'စမ်းသပ်အသုံးပြုပြီး ဖြစ်ပါသည်',
        'trial_success': '🎉 စမ်းသပ်အသုံးပြုခွင့် အောင်မြင်ပါသည်။ သင်၏ အခမဲ့ Key ကို စတင်အသုံးပြုနိုင်ပြီ ဖြစ်ပါသည်။',

        // ── Welcome (no keys yet) ──
        'welcome_title': 'Wavy မှ ကြိုဆိုပါတယ်!',
        'home_empty_title': 'ချိတ်ဆက်အသုံးပြုရန် အဆင်သင့်ဖြစ်ပြီလား',
        'home_empty_desc': 'VPN Key ရယူပြီး အင်တာနက်ကို လွတ်လပ်လုံခြုံစွာ အသုံးပြုလိုက်ပါ။',
        'home_empty_action': '🚀 VPN Key ရယူရန်',
        'welcome_text': 'လူကြီးမင်းတွင် VPN Key မရှိသေးပါ။ အောက်ပါ Plan တစ်ခုကို ရွေးချယ်ပြီး လွယ်ကူလျင်မြန်စွာ စတင်နိုင်ပါသည်။',
        'download_title': 'Key မဝယ်ယူမီ Happ Proxy App အား ထည့်သွင်းထားရန် လိုအပ်ပါသည်',
        'download_text': 'Key ကို အသက်မသွင်းမီ Happ Proxy App ကို Download ပြုလုပ်ပါ။ အခမဲ့ဖြစ်ပြီး အချိန်တိုအတွင်း လုပ်ဆောင်နိုင်ပါသည်။',
        'btn_download': 'App Download ရယူရန်',
        'how_it_works': 'အသုံးပြုနည်း',
        'step_1': 'အောက်ပါ Plan တစ်ခုကို ရွေးချယ်ပါ',
        'step_2': 'Mobile Banking ဖြင့် ငွေလွှဲပါ',
        'step_3': 'Screenshot ပေးပို့ပါ — စက္ကန့်ပိုင်းအတွင်း အတည်ပြုပေးပါမည်',
        'step_4': 'VPN Key ကို စတင်အသုံးပြုနိုင်ပါပြီ!',

        // ── Plans ──
        'loading_plans': 'Plan များကို ရှာဖွေနေပါသည်...',
        'loading_wallet': 'Wallet အချက်အလက်ကို ရယူနေပါသည်...',
        'title_extend': 'Key သက်တမ်းတိုးရန်',
        'title_choose_plan': 'Plan ရွေးချယ်ရန်',
        'title_top_up': 'Wallet သို့ ငွေဖြည့်ရန်',
        'choose_payment_method': 'ငွေပေးချေမှုနည်းလမ်းကို ရွေးချယ်ပါ',
        'create_payment_request': 'ငွေပေးချေမှု တောင်းဆိုမှု ဖန်တီးရန်',
        'pay_with_mobile_banking': 'Mobile banking ဖြင့် ပေးချေမည်',
        'wallet_balance_unavailable': 'Wallet လက်ကျန်ငွေ မရရှိနိုင်ပါ',
        'wallet_balance_low': 'Wallet လက်ကျန်ငွေ မလုံလောက်ပါ',
        'top_up_amount': '{{amount}} {{currency}}',
        'subtitle_extending': 'သက်တမ်းတိုးမည့် Key: {{label}}',
        'subtitle_new_key': 'Plan တစ်ခုကို ရွေးချယ်ပါ — နှိပ်၍ ရှေ့ဆက်ပါ',
        'subtitle_new_key_hint': 'VPN Key အသစ်တစ်ခုကို ဖန်တီးပေးပါမည်',
        'best_value': '✦ အသင့်တော်ဆုံး',
        'plans_load_error': 'Plan များကို မရယူနိုင်ပါ။ ထပ်မံကြိုးစားပါ။',
        'invalid_plan_selected': 'Plan ရွေးချယ်မှု မမှန်ကန်ပါ',
        'unlimited': 'Data ကန့်သတ်ချက် မရှိ',
        'per_day': 'တစ်ရက်လျှင် {{currency}} ကျပ်',
        'new_expiry': '✓ သက်တမ်းကုန်ဆုံးမည့် ရက်အသစ်: {{date}}',

        // ── Plans Help ──
        'help_extend_info': '✅ သက်တမ်းတိုးခြင်းသည် လက်ရှိ ကျန်ရှိနေသော ရက်နှင့် Data ပေါ်တွင် ထပ်မံပေါင်းထည့်ပေးသည် — ဆုံးရှုံးမှု မရှိပါ။ Key နှင့် Setting များမှာလည်း ပြောင်းလဲသွားမည် မဟုတ်ပါ။',
        'help_new_key_info': 'Plan ဝယ်ယူမှုတိုင်းအတွက် VPN Key အသစ်တစ်ခု ရရှိမည်ဖြစ်ပါသည်။ Unlimited Plan တွင် Data ကန့်သတ်ချက် လုံးဝမရှိပါ။ Limited Plan သည် ဈေးနှုန်းပိုမိုသက်သာသော်လည်း လစဉ် Data ကန့်သတ်ချက် ရှိပါသည်။',
        'help_filtered_plans': '{{type}} Plan များကိုသာ ဖော်ပြထားပါသည် — လူကြီးမင်း၏ လက်ရှိ {{type_desc}} Key အမျိုးအစားနှင့် ကိုက်ညီသောကြောင့် ဖြစ်ပါသည်။',
        'help_payments': '💳 Checkout တွင် လက်ရှိအသုံးပြုနိုင်သော mobile banking account များကို ပြပေးပါမည်။ ငွေပမာဏကို အတိအကျလွှဲပြီး Screenshot တင်ပေးရုံဖြင့် ချက်ချင်း အတည်ပြုပေးပါသည်။',

        // ── Checkout ──
        'creating_purchase': 'ကျေးဇူးပြု၍ ခေတ္တစောင့်ဆိုင်းပါ...',
        'payment_details': 'ငွေပေးချေမှု အသေးစိတ်',
        'amount_to_send': 'လွှဲပြောင်းပေးရမည့် ငွေပမာဏ (အတိအကျ)',
        'send_to_phone': 'ဤဖုန်းနံပါတ်သို့ လွှဲပြောင်းပေးရပါမည်',
        'warning_no_note': '⚠️ မှတ်ချက် (Remark/Note) တွင် "Payment" ဟုသာ ရေးသားပေးပါ',
        'upload_btn': '📤 ငွေလွှဲပြေစာ (Screenshot) ပေးပို့ရန်',
        'uploading_btn': 'စစ်ဆေးအတည်ပြုနေပါသည်...',
        'upload_hint': 'ငွေလွှဲပြောင်းမှု အောင်မြင်ပြီးနောက် အတည်ပြုချက်ပြသော စာမျက်နှာကို Screenshot ရှင်းလင်းစွာရိုက်၍ ဤနေရာတွင် ပေးပို့ပါ။ စက္ကန့်ပိုင်းအတွင်း အလိုအလျောက် စစ်ဆေးပေးမည် ဖြစ်ပါသည်။',

        // ── Checkout Guide Steps ──
        'guide_title': 'ငွေပေးချေနည်း အဆင့်ဆင့်',
        'guide_step_1': 'Mobile Banking App အား ဖွင့်ပါ',
        'guide_step_1_hint': 'လက်ရှိအသုံးပြုနိုင်သော account များ: {{methods}}',

        'guide_step_2': 'ရွေးချယ်ထားသော account သို့ ငွေပမာဏကို အတိအကျ လွှဲပြောင်းပါ',
        'label_amount': 'ငွေပမာဏ (ကျပ်)',
        'label_phone': 'ဖုန်းနံပါတ်',
        'label_account_name': 'Account အမည်',

        'guide_step_3': 'မှတ်ချက် (Remark/Note) တွင် "Payment" ဟုသာ ရေးသားပါ',
        'guide_step_3_hint': 'VPN, Wavy, Outline အစရှိသည့် စကားလုံးများ လုံးဝ မထည့်သွင်းရပါ',

        'guide_step_4': 'ငွေလွှဲပြေစာကို Screenshot ရိုက်၍ အောက်တွင် ပေးပို့ပါ',
        'guide_step_4_hint': 'စက္ကန့်ပိုင်းအတွင်း အလိုအလျောက် အတည်ပြုပေးပါမည်',

        'tap_to_copy': 'နှိပ်၍ ကူးယူပါ',
        'copied': 'ကူးယူပြီးပါပြီ ✓',
        'important_warning': '⚠️ အထူးသတိပြုရန်: ပြသထားသော ငွေပမာဏကိုသာ အတိအကျ လွှဲပြောင်းပေးပါ။ မှတ်ချက် (Remark/Note) တွင် "Payment" ဟုသာ ရေးသားပေးရန် အထူးမေတ္တာရပ်ခံအပ်ပါသည်။ အခြားစကားလုံးများ (VPN, Wavy, Outline) ထည့်သွင်းထားပါက စစ်ဆေးရန် ခက်ခဲနိုင်ပြီး Key ရရှိမည်မဟုတ်ပါ။',

        // ── Success / Error ──
        'success_title': '✅ ငွေပေးချေမှု အောင်မြင်ပါသည်',
        'success_extend': 'Key သက်တမ်းတိုးခြင်း အောင်မြင်ပါသည်။ ရက်နှင့် Data များကို ထပ်မံပေါင်းထည့်ပေးလိုက်ပြီ ဖြစ်ပါသည်။',
        'success_new': 'သင်၏ VPN Key အသစ် အသင့်ဖြစ်ပါပြီ။ အောက်ပါခလုတ်ကို နှိပ်၍ Happ Proxy သို့ ထည့်သွင်းပါ။',
        'success_tip_extend': 'နောက်သို့ ပြန်ထွက်၍ Key ၏ သက်တမ်းကုန်ဆုံးရက်နှင့် ကျန်ရှိ Data ကို စစ်ဆေးနိုင်ပါသည်။',
        'success_tip_new': 'အောက်ပါ "Happ Proxy သို့ ဝင်ရောက်ရန်" ကို နှိပ်၍ VPN အား ချက်ချင်း စတင်ခလုပ်နှိပ်၍ အသုံးပြုနိုင်ပါသည်။ သို့မဟုတ် Key လင့်ခ်ကို အခြား VPN App တွင် ထည့်သွင်းအသုံးပြုနိုင်ပါသည်။',
        'verify_error_tip': '💡 ဖြေရှင်းရန် အကြံပြုချက်: လွှဲပြောင်းသည့် ငွေပမာဏနှင့် receiver ၏ ဖုန်းနံပါတ် သို့မဟုတ် account name တို့ကို စစ်ဆေးပါ။ ထို့ပြင် Screenshot သည် (လက်ကျန်ငွေပြသော ပုံမဟုတ်ဘဲ) Transaction အတည်ပြုပြေစာ ဖြစ်ရန် လိုအပ်ပါသည်။',

        // ── Wallet ──
        'wallet_title': 'Wavy Wallet',
        'wallet_subtitle': 'ငွေစာရင်း လက်ကျန် နှင့် အလိုအလျောက် သက်တမ်းတိုးခြင်း',
        'current_balance': 'လက်ရှိ လက်ကျန်ငွေ',
        'top_up_wallet': '+ ငွေဖြည့်သွင်းရန်',
        'wallet_error': 'Wallet အချက်အလက်ကို မရရှိနိုင်ပါ။ ထပ်မံကြိုးစားပါ။',
        'auto_renew_title': 'အလိုအလျောက် သက်တမ်းတိုးစနစ် (Auto-Renewal)',
        'auto_renew_enabled': 'ဖွင့်ထားသည် — Key သက်တမ်းကုန်ဆုံးပါက အလိုအလျောက် သက်တမ်းတိုးပေးမည်',
        'auto_renew_disabled': 'ပိတ်ထားသည် — ဖွင့်ရန် နှိပ်ပါ',
        'transaction_history': 'ငွေစာရင်း မှတ်တမ်းများ',
        'no_transactions': 'မှတ်တမ်း မရှိသေးပါ',
        'transaction_topup': 'Wallet အတွင်းသို့ ငွေဖြည့်သွင်းခြင်း',
        'transaction_purchase': 'Plan ဝယ်ယူခြင်း',
        'transaction_refund': 'ငွေပြန်အမ်းခြင်း',
        'wallet_info': 'Wallet အတွင်းရှိ ငွေများသည် သက်တမ်းကုန်ဆုံးခြင်း မရှိဘဲ မည်သည့် Plan အတွက်မဆို အသုံးပြုနိုင်ပါသည်။',
        'no_refund_title': 'ငွေပြန်အမ်းနိုင်မှု ဆိုင်ရာ မူဝါဒ',
        'no_refund_desc': 'Plan ဝယ်ယူပြီးပါက ငွေပြန်အမ်းခြင်း ပြုလုပ်ပေးမည် မဟုတ်ပါ။ သို့သော် Wallet အတွင်းရှိ လက်ကျန်ငွေများသည် သက်တမ်းကုန်ဆုံးခြင်း မရှိပါ။',

        // ── Referral ──
        'referral_earnings': '🤝 ဖိတ်ခေါ်မှုမှ ရရှိငွေများ',
        'friends_invited': 'ဖိတ်ခေါ်ထားသော သူငယ်ချင်းများ:',
        'total_earned': 'စုစုပေါင်း ရရှိသောငွေ:',
        'share_link': 'လင့်ခ် မျှဝေရန် →',
        'referral_pending': 'စောင့်ဆိုင်းဆဲ',
        'referral_bonus_received': 'အပိုဆုငွေ ရရှိပြီး',
        'referral_share_text': 'ဒီ Link ကနေ Wavy VPN ဝယ်ရင် ၁,၀၀၀ ကျပ် အခမဲ့ရမှာနော် 🎁',
        'referral_wallet_chip': 'ဖိတ်ခေါ်မှု ဉီးရေ {{count}} · ရရှိငွေ: {{earned}} ကျပ်',
        'referral_checkout_title': '🎁 အပြန်အလှန် လက်ဆောင်များ ရယူရန်',
        'referral_checkout_desc': 'သူငယ်ချင်းများကို Wavy သို့ ဖိတ်ခေါ်ပြီး နှစ်ဦးစလုံး အခမဲ့ VPN သုံးစွဲခွင့်ကို ရယူလိုက်ပါ!',
        'referral_checkout_btn': 'လင့်ခ် မျှဝေရန်',

        // ── Wallet Tips ──
        'wallet_tips_title': 'Wallet ကို အသုံးပြုခြင်း၏ အားသာချက်များ',
        'wallet_tip_1_title': 'အဆက်မပြတ် ဝန်ဆောင်မှု',
        'wallet_tip_1_desc': 'အလိုအလျောက် သက်တမ်းတိုးစနစ်ကို ဖွင့်ထားခြင်းဖြင့် သက်တမ်းကုန်ဆုံးချိန်တွင် အင်တာနက် ပြတ်တောက်မှု မရှိဘဲ အလိုအလျောက် သက်တမ်းတိုးပေးမည် ဖြစ်ပါသည်။',
        'wallet_tip_2_title': 'ချက်ချင်း အသက်ဝင်မည်',
        'wallet_tip_2_desc': 'Screenshot ပေးပို့ပြီး စောင့်ဆိုင်းရန် မလိုအပ်ပါ။ Wallet မှ ငွေပေးချေမှုသည် စက္ကန့်ပိုင်းအတွင်း ပြီးမြောက်ပြီး Key ကို ချက်ချင်း ရရှိမည် ဖြစ်ပါသည်။',
        'wallet_tip_3_title': 'လွယ်ကူ လုံခြုံမှု',
        'wallet_tip_3_desc': 'တစ်ကြိမ် ငွေဖြည့်ရုံဖြင့် အချိန်ကြာမြင့်စွာ အသုံးပြုနိုင်မည်ဖြစ်ပြီး၊ သက်တမ်းကုန်တိုင်း Bank App ဖွင့်၍ ငွေလွှဲရန် မလိုအပ်တော့ပါ။',

        // ── Wallet Payment ──
        'pay_with_wallet': '⚡ Wallet ဖြင့် ငွေပေးချေရန်',
        'your_balance': 'လူကြီးမင်း၏ လက်ကျန်ငွေ:',
        'or_pay_manually': 'သို့မဟုတ် Mobile Banking ဖြင့် ကိုယ်တိုင် ငွေလွှဲရန်:',
        'accepted_methods': '{{methods}}',
        'wallet_pay_success': 'Wallet ဖြင့် အောင်မြင်စွာ ပေးချေပြီးပါပြီ',
        'check_home_for_key': 'သင်၏ Key မှာ အသက်ဝင်နေပြီဖြစ်ပါသည်။ "ကျွန်ုပ်၏ Key များ" တွင် ဝင်ရောက်ကြည့်ရှုနိုင်ပါသည်။',
        'funds_added': 'Wallet အတွင်းသို့ ငွေဖြည့်သွင်းခြင်း အောင်မြင်ပါသည်။',
        'wallet_pay_processing': 'ဆောင်ရွက်နေပါသည်...',
        'wallet_pay_btn': 'Wallet မှ {{amount}} {{currency}} ဖြတ်တောက်ရန်',
        'wallet_pay_error': 'Wallet မှ ငွေပေးချေမှု မအောင်မြင်ပါ။ ကျေးဇူးပြု၍ လက်ကျန်ငွေကို စစ်ဆေးပြီး ထပ်မံကြိုးစားပါ။',

        // ── Wallet Top-up Success ──
        'success_topup_desc': 'Wallet အတွင်းသို့ ငွေဖြည့်သွင်းခြင်း အောင်မြင်ပါသည်။',
        'back_to_wallet': 'Wallet သို့ ပြန်သွားရန်',

        // ── Wallet Empty States ──
        'wallet_empty_title': 'မှတ်တမ်း မရှိသေးပါ',
        'wallet_empty_desc': 'ပထမဆုံး ငွေဖြည့်သွင်းမှု သို့မဟုတ် ဝယ်ယူမှု ပြုလုပ်ပြီးသည်နှင့် မှတ်တမ်းများ ဤနေရာတွင် ပေါ်လာမည် ဖြစ်ပါသည်။',

        // ── Promo Code ──
        'promo_placeholder': 'Promo code ထည့်သွင်းရန်',
        'promo_apply': 'အသုံးပြုရန်',
        'promo_validating': 'စစ်ဆေးနေပါသည်...',
        'promo_valid': '🎉 Code အတည်ပြုပြီးဖြစ်ပါသည်။ {{percent}}% လျှော့ဈေး ရရှိပါမည်!',
        'promo_invalid': '❌ Code မှားယွင်းနေသည် သို့မဟုတ် သက်တမ်းကုန်ဆုံးသွားပါပြီ',
    }
};
