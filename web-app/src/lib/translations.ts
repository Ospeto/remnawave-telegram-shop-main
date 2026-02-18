export type Language = 'en' | 'my';

export const translations: Record<Language, Record<string, string>> = {
    en: {
        // General
        'loading': 'Loading...',
        'retry': 'Retry',
        'error_prefix': 'Error: ',
        'powered_by': 'Powered by Wavy Private Server',
        'open_in_tg': 'Open this app inside Telegram',

        // Navigation / Steps
        'nav_plan': 'Plan',
        'nav_payment': 'Payment',
        'nav_verify': 'Verify',
        'back_button': 'Back',
        'back_to_plans': 'Back to Plans',
        'go_home': 'Go to My Keys',
        'btn_open_happ': 'Open in Happ',
        'success_happ_hint': 'Tap to auto-import your key into the Happ Proxy app',

        // Home
        'home_title': 'Wavy Private Server',
        'active_key_count': '{{count}} active key',
        'active_key_count_plural': '{{count}} active keys',
        'no_active_keys': 'No active keys',
        'key_active': '● Active',
        'key_expired': '○ Expired',
        'days_left': 'days left',
        'expires_on': 'Expires {{date}}',
        'data_usage': 'Data Usage',
        'btn_add_happ': 'Add to Happ',
        'btn_extend': 'Extend',
        'btn_copy_key': 'Copy Key',
        'btn_buy_new': 'Buy New Key',
        'btn_get_started': 'Get Started — Buy a Plan',

        // Home Help
        'help_expired': 'This key has expired. Tap Extend below to add more days and keep using it.',
        'help_btn_add': 'Add to Happ — auto-imports this key into the Happ Proxy app',
        'help_btn_extend': 'Extend — add more days & data to this key',
        'help_btn_copy': 'copy link to use in any VPN app',
        'tip_multi_key': 'You can buy multiple keys for different devices, or extend an existing one.',
        'info_device_limit': 'Devices per Key',
        'info_device_count': '3',
        'info_servers': 'Server Locations',
        'info_server_list': 'Thailand · Germany · US · Singapore · Japan',
        'contact_support': 'Contact Support',

        // Trial
        'trial_button': '🎁 Get Free Trial ({{days}} Days)',
        'trial_activating': 'Activating trial...',
        'trial_error': 'Failed to activate trial. Please try again.',
        'trial_success': 'Trial activated! Your free key is ready.',

        // Welcome
        'welcome_title': 'Welcome!',
        'welcome_text': 'You don\'t have any VPN keys yet. Buy a plan to get started — it only takes a minute!',
        'download_title': 'Download Happ Proxy',
        'download_text': 'You need to download the Happ Proxy app first to use the key.',
        'btn_download': '📥 Download App',
        'how_it_works': 'How it works',
        'step_1': 'Choose a plan below',
        'step_2': 'Pay via mobile banking',
        'step_3': 'Upload a screenshot — we verify it instantly',
        'step_4': 'Start using your VPN key!',

        // Plans
        'loading_plans': 'Loading plans...',
        'title_extend': 'Extend Key',
        'title_choose_plan': 'Choose a Plan',
        'subtitle_extending': 'Extending: {{label}}',
        'subtitle_new_key': 'Pick a plan that fits your needs — tap to continue',
        'subtitle_new_key_hint': 'A new subscription key will be created',
        'best_value': 'Best Value',
        'unlimited': 'Unlimited',
        'per_day': '/{{currency}}/day',
        'new_expiry': '✓ New expiry: {{date}}',

        // Plans Help
        'help_extend_info': 'Extending adds more days and data to your existing key. Your current remaining days and data are kept — nothing is lost!',
        'help_new_key_info': 'Each plan gives you a new VPN key with a set number of days and data. Unlimited plans have no data cap. Limited plans are cheaper but have a monthly data limit.',
        'help_filtered_plans': 'Only {{type}} plans are shown because your key is {{type_desc}} plan.',
        'help_payments': 'Accepted payments: KPay · Wave · AYA Pay. After selecting a plan, you\'ll send money and upload a screenshot for instant verification.',

        // Checkout
        'creating_purchase': 'Creating purchase...',
        'payment_details': 'Payment Details',
        'amount_to_send': 'Amount to Send',
        'send_to_phone': 'Send to Phone Number',
        'warning_no_note': '⚠️ Write only "Payment" in the note/remark field',
        'upload_btn': 'Upload Payment Screenshot',
        'uploading_btn': 'Verifying your payment...',
        'upload_hint': 'After you\'ve sent the money, take a screenshot of the confirmation and upload it here. We\'ll verify it automatically in seconds.',

        // Checkout Guide
        'guide_title': 'Follow these steps',
        'guide_step_1': 'Open your banking app',
        'guide_step_1_hint': 'KPay, Wave, or AYA Pay',
        'guide_step_2': 'Send this exact amount',
        'guide_step_3': 'To this phone number',
        'tap_to_copy': 'Tap to copy',
        'copied': 'Copied!',
        'guide_step_4': 'Write "Payment" in the note/remark',
        'guide_step_4_hint': 'Do NOT write "VPN", "Outline", or anything else — just "Payment"',
        'important_warning': 'Important: Send the exact amount shown above. In the note/remark field, write only "Payment". Do NOT write "VPN", "Outline", or anything else — your key will not be issued.',

        // Success / Fail
        'success_title': 'Payment Verified!',
        'success_extend': 'Your key has been extended successfully! Extra days and data have been added.',
        'success_new': 'Your new VPN key has been created and is ready to use.',
        'success_tip_extend': 'Go back to see your updated key with the new expiry date.',
        'success_tip_new': 'Go back to your keys and tap "Add to Happ" to start using your VPN, or copy the link for any VPN app.',
        'verify_error_tip': 'Double-check that the amount and phone number match exactly. Make sure the screenshot clearly shows the transaction details.',

        // Wallet
        'wallet_title': 'My Wallet',
        'wallet_subtitle': 'Manage your balance & subscriptions',
        'current_balance': 'Current Balance',
        'top_up_wallet': 'Top Up Wallet',
        'auto_renew_title': 'Auto-Renewal',
        'auto_renew_enabled': 'On - Renews every {{days}} days',
        'auto_renew_disabled': 'Off - Tap to enable',
        'transaction_history': 'Transaction History',
        'no_transactions': 'No transactions yet',
        'transaction_topup': 'Wallet Top-up',
        'transaction_purchase': 'Plan Purchase',
        'transaction_refund': 'Refund',
        'wallet_info': 'Funds in your wallet never expire and can be used for any future purchase.',

        // Wallet Tips
        'wallet_tips_title': 'Why use Wallet?',
        'wallet_tip_1_title': 'Never Get Disconnected',
        'wallet_tip_1_desc': 'Enable auto-renew to keep your VPN running 24/7 without interruption.',
        'wallet_tip_2_title': 'Instant Activation',
        'wallet_tip_2_desc': 'Skip the screenshot verification. Wallet payments are approved instantly.',
        'wallet_tip_3_title': 'Save Time',
        'wallet_tip_3_desc': 'Top up once, pay for months. No need to open your banking app every time.',
    },
    my: {
        // General
        'loading': 'ခေတ္တစောင့်ပါ...',
        'retry': 'ထပ်ကြိုးစားမည်',
        'error_prefix': 'Error: ',
        'powered_by': 'Wavy Private Server မှ ပံ့ပိုးသည်',
        'open_in_tg': 'Telegram အတွင်း ဖွင့်ပါ',

        // Navigation / Steps
        'nav_plan': 'Plan',
        'nav_payment': 'Payment',
        'nav_verify': 'Verify',
        'back_button': 'နောက်သို့',
        'back_to_plans': 'Plan များသို့',
        'go_home': 'ကျွန်ုပ်၏ Key များသို့',
        'btn_open_happ': 'Happ ထဲသို့ ထည့်မည်',
        'success_happ_hint': 'Happ Proxy app ထဲ Key အလိုအလျောက် ထည့်ရန် နှိပ်ပါ',

        // Home
        'home_title': 'Wavy Private Server',
        'active_key_count': 'သက်တမ်းရှိနေသော Key ၁ ခု',
        'active_key_count_plural': 'သက်တမ်းရှိနေသော Key {{count}} ခု',
        'no_active_keys': 'သက်တမ်းရှိ Key မရှိပါ',
        'key_active': '● Active',
        'key_expired': '○ Expired',
        'days_left': 'ရက်ကျန်',
        'expires_on': 'သက်တမ်းကုန်မည့်ရက် {{date}}',
        'data_usage': 'Data သုံးစွဲမှု',
        'btn_add_happ': 'Happ သို့ ထည့်မည်',
        'btn_extend': 'သက်တမ်းတိုးမည်',
        'btn_copy_key': 'Key ကူးမည်',
        'btn_buy_new': 'Key အသစ်၀ယ်မည်',
        'btn_get_started': 'စတင်လိုက်ပါ — Plan ရွေးမည်',

        // Home Help
        'help_expired': 'Key Expire ဖြစ်သွားပါပြီ။ ဆက်လက်အသုံးပြုရန် အောက်ပါ Extend ခလုတ်ကို နှိပ်ပါ။',
        'help_btn_add': 'Happ သို့ ထည့်မည် — Happ Proxy app ထဲသို့ အလိုအလျောက် ထည့်သွင်းပေးမည်',
        'help_btn_extend': 'Extend — ရက်နှင့် Data ပမာဏ ထပ်ပေါင်းထည့်မည်',
        'help_btn_copy': 'VPN app တခုခုတွင်သုံးရန် link ကူးယူမည်',
        'tip_multi_key': 'device အမျိုးမျိုးအတွက် Key အများကြီး ဝယ်ထားနိုင်သလို၊ ရှိပြီးသား Key ကိုလည်း Extend လုပ်နိုင်ပါသည်။',
        'info_device_limit': 'Devices per Key',
        'info_device_count': '3',
        'info_servers': 'Server Locations',
        'info_server_list': 'Thailand · Germany · US · Singapore · Japan',
        'contact_support': 'Contact Support',

        // Trial
        'trial_button': '🎁 အခမဲ့ ရက် {{days}} ရက် စမ်းသုံးကြည့်ပါ',
        'trial_activating': 'Trial ဖွင့်နေသည်...',
        'trial_error': 'Trial ဖွင့်၍ မရပါ။ ထပ်ကြိုးစားပါ။',
        'trial_success': 'Trial ဖွင့်ပြီး! အခမဲ့ Key အဆင်သင့်ဖြစ်ပါပြီ။',

        // Welcome
        'welcome_title': 'မင်္ဂလာပါ!',
        'welcome_text': 'သင့်တွင် VPN key မရှိသေးပါ။ စတင်အသုံးပြုရန် Plan တစ်ခုကို ဝယ်ယူပါ — ၁ မိနစ်သာ ကြာပါမည်!',
        'download_title': 'Happ Proxy ကို Download ရယူပါ',
        'download_text': 'VPN key အသုံးမပြုမီ Happ Proxy app ကို အရင် Download လုပ်ထားရန် လိုအပ်ပါသည်။',
        'btn_download': 'App ကို Download လုပ်မည်',
        'how_it_works': 'အလုပ်လုပ်ပုံ',
        'step_1': 'အောက်တွင် Plan တစ်ခု ရွေးပါ',
        'step_2': 'Mobile Banking ဖြင့် ငွေလွှဲပါ',
        'step_3': 'ငွေလွှဲပြေစာ (Screenshot) တင်ပါ — ချက်ချင်း စစ်ဆေးပေးသည်',
        'step_4': 'VPN key စတင် အသုံးပြုနိုင်ပါပြီ!',

        // Plans
        'loading_plans': 'Plan များ ရယူနေသည်...',
        'title_extend': 'သက်တမ်းတိုး',
        'title_choose_plan': 'Plan ရွေးချယ်ပါ',
        'subtitle_extending': 'Extend လုပ်နေသည်: {{label}}',
        'subtitle_new_key': 'သင့်လိုအပ်ချက်နှင့် ကိုက်ညီမည့် Plan ကို ရွေးပါ',
        'subtitle_new_key_hint': 'Subscription key အသစ် ရရှိပါမည်',
        'best_value': 'အတန်ဆုံး။',
        'unlimited': 'Unlimited',
        'per_day': '/{{currency}}/ရက်',
        'new_expiry': '✓ သက်တမ်းကုန်မည့်ရက်သစ်: {{date}}',

        // Plans Help
        'help_extend_info': 'Extend လုပ်ခြင်းသည် ရှိပြီးသား Key တွင် ရက်နှင့် Data ပမာဏကို ပေါင်းထည့်ပေးသည်။ လက်ကျန် ရက်နှင့် Data များ ဆုံးရှုံးမည် မဟုတ်ပါ!',
        'help_new_key_info': 'Plan သစ် ဝယ်ယူတိုင်း VPN key အသစ်တစ်ခု ရရှိပါမည်။ Unlimited Plan များတွင် Data ကန့်သတ်ချက် မရှိပါ။ Limited Plan များသည် ဈေးသက်သာသော်လည်း Data ကန့်သတ်ချက် ရှိပါသည်။',
        'help_filtered_plans': 'သင်၏ Key သည် {{type_desc}} ဖြစ်သောကြောင့် {{type}} Plan များကိုသာ ပြထားပါသည်။',
        'help_payments': 'လက်ခံသော ငွေပေးချေမှုများ: KPay · Wave · AYA Pay။ Plan ရွေးပြီးပါက ငွေလွှဲပြီး Screenshot တင်ကာ အသုံးပြုနိုင်ပါသည်။',

        // Checkout
        'creating_purchase': 'အော်ဒါ တင်နေသည်...',
        'payment_details': 'ငွေပေးချေမှု အချက်အလက်',
        'amount_to_send': 'လွှဲရမည့် ပမာဏ',
        'send_to_phone': 'ငွေလက်ခံမည့် ဖုန်းနံပါတ်',
        'warning_no_note': '⚠️ Note/Remark တွင် "Payment" ဟုသာ ရေးပါ',
        'upload_btn': 'ငွေလွှဲပြေစာ တင်မည်',
        'uploading_btn': 'စစ်ဆေးနေသည်...',
        'upload_hint': 'ငွေလွှဲပြီးပါက Transaction ပြေစာ Screenshot ကို ရိုက်ပြီး ဤနေရာတွင် တင်ပေးပါ။ စက္ကန့်ပိုင်းအတွင်း အလိုအလျောက် စစ်ဆေးပေးပါမည်။',

        // Checkout Guide
        'guide_title': 'ဤအဆင့်များအတိုင်း လုပ်ဆောင်ပါ',
        'guide_step_1': 'သင့် Banking App ဖွင့်ပါ',
        'guide_step_1_hint': 'KPay, Wave, သို့မဟုတ် AYA Pay',
        'guide_step_2': 'ဤပမာဏ အတိအကျ လွှဲပါ',
        'guide_step_3': 'ဤဖုန်းနံပါတ်သို့ လွှဲပါ',
        'tap_to_copy': 'ဖိပီး ကူးယူပါ',
        'copied': 'ကူးယူပြီး!',
        'guide_step_4': 'Note/Remark တွင် "Payment" ဟုသာ ရေးပါ',
        "guide_step_4_hint": '"VPN", "Outline" မရေးပါနှင့်။ "Payment" ဟုသာ ရေးပါ — အခြားစာသားများ ရေးပါက Key ထုတ်ပေးမည် မဟုတ်ပါ။',
        'important_warning': 'အရေးကြီးသည်: ပြထားသော ပမာဏ အတိအကျ လွှဲပါ။ Note/Remark တွင် "Payment" ဟုသာ ရေးပါ။ "VPN", "Outline" ဟု ရေးပါက Key ထုတ်ပေးမည် မဟုတ်ပါ။',

        // Success / Fail
        'success_title': 'ငွေပေးချေမှု အောင်မြင်သည်!',
        'success_extend': 'Extend လုပ်ခြင်း အောင်မြင်ပါသည်။ ရက်နှင့် Data များ ပေါင်းထည့်လိုက်ပါပြီ။',
        'success_new': 'VPN key အသစ် ရရှိပါပြီ။ စတင် အသုံးပြုနိုင်ပါပြီ။',
        'success_tip_extend': 'နောက်သို့ ပြန်သွားပြီး သက်တမ်းရက် အသစ်ကို စစ်ဆေးနိုင်ပါသည်။',
        'success_tip_new': 'နောက်သို့ ပြန်သွားပြီး "Happ သို့ ထည့်မည်" ကို နှိပ်၍ အသုံးပြုနိုင်ပါပြီ။',
        'verify_error_tip': 'ပမာဏနှင့် ဖုန်းနံပါတ် တူညီမှု ရှိမရှိ ပြန်စစ်ပါ။ Screenshot တွင် Transaction အချက်အလက်များ ထင်ရှားမှု ရှိမရှိ စစ်ဆေးပါ။',

        // Wallet
        'wallet_title': 'ကျွန်ုပ်၏ Wallet',
        'wallet_subtitle': 'ငွေဖြည့်ခြင်းနှင့် Auto-Renewal ဝန်ဆောင်မှု',
        'current_balance': 'လက်ကျန်ငွေ',
        'top_up_wallet': 'ငွေဖြည့်မည်',
        'auto_renew_title': 'အလိုအလျောက် သက်တမ်းတိုးခြင်း',
        'auto_renew_enabled': 'ဖွင့်ထားသည် - {{days}} ရက်တိုင်း သက်တမ်းတိုးမည်',
        'auto_renew_disabled': 'ပိတ်ထားသည် - ဖွင့်ရန် နှိပ်ပါ',
        'transaction_history': 'အသုံးပြုမှု မှတ်တမ်း',
        'no_transactions': 'မှတ်တမ်း မရှိသေးပါ',
        'transaction_topup': 'Wallet ငွေဖြည့်',
        'transaction_purchase': 'Plan ဝယ်ယူမှု',
        'transaction_refund': 'ငွေပြန်အမ်းမှု',
        'wallet_info': 'Wallet အတွင်းရှိငွေသည် သက်တမ်းကုန်ဆုံးခြင်း မရှိပါ။',

        // Wallet Tips
        'wallet_tips_title': 'Wallet ကို ဘာကြောင့် သုံးသင့်သလဲ?',
        'wallet_tip_1_title': 'လိုင်းပြတ်တောက်မှု မရှိတော့ပါ',
        'wallet_tip_1_desc': 'Auto-renewal ဖွင့်ထားပါက သက်တမ်းကုန်ခါနီးတိုင်း အလိုအလျောက် သက်တမ်းတိုးပေးသဖြင့် 24/7 အသုံးပြုနိုင်ပါသည်။',
        'wallet_tip_2_title': 'မိနစ်ပိုင်းအတွင်း ရရှိမည်',
        'wallet_tip_2_desc': 'Screenshot တင်ပြီး စောင့်ဆိုင်းနေရန် မလိုတော့ပါ။ Wallet ဖြင့် ဝယ်ပါက Key ချက်ချင်း ရရှိပါသည်။',
        'wallet_tip_3_title': 'အချိန်ကုန် သက်သာသည်',
        'wallet_tip_3_desc': 'တစ်ခါတည်း ငွေဖြည့်ထားပြီး လပေါင်းများစွာ အသုံးပြုနိုင်ပါသည်။ ခဏခဏ ငွေလွှဲနေရန် မလိုတော့ပါ။',
    }
};

