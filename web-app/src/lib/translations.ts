export type Language = 'en' | 'my';

export const translations: Record<Language, Record<string, string>> = {
    en: {
        // ── General ──
        'loading': 'Loading...',
        'retry': 'Try Again',
        'error_prefix': 'Error: ',
        'powered_by': 'Wavy Private Server',
        'open_in_tg': 'Open inside Telegram to use this app',

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
        'step_2': 'Pay via KPay, Wave, or AYA Pay',
        'step_3': 'Upload a screenshot — verified in seconds',
        'step_4': 'Your VPN key is ready!',

        // ── Plans ──
        'loading_plans': 'Fetching plans...',
        'title_extend': 'Extend Your Key',
        'title_choose_plan': 'Choose a Plan',
        'title_top_up': 'Top Up Wallet',
        'top_up_amount': '{{amount}} {{currency}}',
        'subtitle_extending': 'Extending: {{label}}',
        'subtitle_new_key': 'Choose a plan — tap to continue',
        'subtitle_new_key_hint': 'A new VPN key will be created for you',
        'best_value': '✦ Best Value',
        'unlimited': 'Unlimited Data',
        'per_day': '{{currency}} MMK/day',
        'new_expiry': '✓ New expiry: {{date}}',

        // ── Plans Help ──
        'help_extend_info': '✅ Extending adds more days and data on top of what you have left — nothing is lost. Your current key and settings stay the same.',
        'help_new_key_info': 'Each plan gives you a dedicated VPN key. Unlimited plans have no data cap. Limited plans cost less but have a monthly data limit — great for light use.',
        'help_filtered_plans': 'Showing {{type}} plans only — matches your current {{type_desc}} key type.',
        'help_payments': '💳 Accepted: KPay · Wave · AYA Pay. After selecting a plan, you\'ll make a transfer and upload a screenshot for instant automated verification.',

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
        'guide_step_1_hint': 'KPay, Wave Money, or AYA Pay',

        'guide_step_2': 'Transfer exact amount to this number',
        'label_amount': 'Amount (MMK)',
        'label_phone': 'Phone',

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
        'verify_error_tip': '💡 Common fixes: make sure the amount and phone number match exactly, and that the screenshot clearly shows the transaction confirmation (not just a balance screen).',

        // ── Wallet ──
        'wallet_title': 'Wavy Wallet',
        'wallet_subtitle': 'Balance & auto-renewal',
        'current_balance': 'Available Balance',
        'top_up_wallet': '+ Top Up Balance',
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
        'referral_wallet_chip': '{{count}} referral · {{earned}} earned →',
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
        'accepted_methods': 'KPay · Wave · AYA Pay',
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
        'loading': 'ခေတ္တစောင့်ပါ...',
        'retry': 'ထပ်ကြိုးစားမည်',
        'error_prefix': 'အမှား: ',
        'powered_by': 'Wavy Private Server',
        'open_in_tg': 'ဤ App ကို Telegram တွင်သာ ဖွင့်အသုံးပြုနိုင်ပါသည်',

        // ── Navigation ──
        'nav_plan': 'Plan',
        'nav_payment': 'ငွေပေးချေမှု',
        'nav_verify': 'အတည်ပြုမှု',
        'back_button': 'နောက်သို့',
        'back_to_plans': 'Plan များသို့ ပြန်သွားမည်',
        'go_home': 'ကျွန်ုပ်၏ Key များ',
        'btn_open_happ': 'Happ Proxy တွင် ဖွင့်မည်',
        'success_happ_hint': 'နှိပ်ပါ — Happ Proxy App ထဲသို့ Key ကို ချက်ချင်း ထည့်သွင်းပေးမည်',

        // ── Home ──
        'home_title': 'Wavy Private Server',
        'active_key_count': 'အသုံးပြုဆဲ Key ၁ ခု',
        'active_key_count_plural': 'အသုံးပြုဆဲ Key {{count}} ခု',
        'no_active_keys': 'အသုံးပြုဆဲ Key မရှိပါ',
        'key_active': '● Active',
        'key_expired': '○ သက်တမ်းကုန်ဆုံး',
        'days_left': 'ရက်ကျန်',
        'expires_on': 'သက်တမ်းကုန်မည့် ရက်: {{date}}',
        'data_usage': 'Data သုံးစွဲမှုပမာဏ',
        'btn_add_happ': 'Happ သို့ ထည့်မည်',
        'btn_extend': 'သက်တမ်းတိုးမည်',
        'btn_copy_key': 'Key ကူးယူမည်',
        'btn_buy_new': 'Key အသစ် ဝယ်မည်',
        'btn_get_started': 'စတင်မည်',

        // ── Home Help / Tips ──
        'help_expired': '⏳ ဤ Key သက်တမ်းကုန်ဆုံးသွားပါပြီ။ "သက်တမ်းတိုးမည်" ကို နှိပ်ပြီး ချက်ချင်း ပြန်လည်ချိတ်ဆက်နိုင်ပါသည်။',
        'help_btn_add': 'Happ သို့ ထည့်မည် — Happ Proxy App ထဲသို့ Key ကို တစ်ချက်နှိပ်ရုံဖြင့် ထည့်သွင်းပေးပါသည်',
        'help_btn_extend': 'သက်တမ်းတိုးမည် — ရှိနေသော ရက်နှင့် Data ပေါ်တွင် ထပ်ပေါင်းထည့်ပါသည်၊ ဆုံးရှုံးမှု မရှိပါ',
        'help_btn_copy': 'Key ကူးယူမည် — Outline/Shadowsocks ကို ထောက်ပံ့သော VPN App မည်သည်မဆို သုံးနိုင်ပါသည်',
        'tip_multi_key': 'အကြံပြုချက်: Device ၃ ခုထက်ပို၍ သုံးလိုပါက Key အသစ်ဝယ်ယူနိုင်သလို၊ ရှိပြီးသား Key ကိုလည်း သက်တမ်းတိုးနိုင်ပါသည်။',
        'info_device_limit': 'Key တစ်ခုလျှင် Device အရေအတွက်',
        'info_device_count': '၃ ခု',
        'info_servers': 'ဆာဗာ တည်နေရာများ',
        'info_server_list': 'TH · DE · US · SG · JP',
        'contact_support': 'အကူအညီ ရယူရန်',

        // ── Trial ──
        'trial_button': '🎁 အခမဲ့ {{days}} ရက် စမ်းသုံးခွင့် ရယူမည်',
        'trial_activating': 'Trial ဖွင့်ပေးနေပါသည်...',
        'trial_error': 'Trial ရယူမရနိုင်ပါ။ ထပ်ကြိုးစားပါ သို့မဟုတ် Support သို့ ဆက်သွယ်ပါ။',
        'trial_success': '🎉 Trial အောင်မြင်ပါသည်! သင်၏ အခမဲ့ Key အဆင်သင့်ဖြစ်ပါပြီ။',

        // ── Welcome (no keys yet) ──
        'welcome_title': 'Wavy မှ ကြိုဆိုပါတယ်!',
        'home_empty_title': 'ချိတ်ဆက်ရန် အဆင်သင့်ဖြစ်ပြီလား?',
        'home_empty_desc': 'VPN Key ရယူပြီး လွတ်လပ်လုံခြုံစွာ အင်တာနက်သုံးစွဲလိုက်ပါ။',
        'home_empty_action': '🚀 Key ရယူမည်',
        'welcome_text': 'သင့်တွင် VPN Key မရှိသေးပါ။ အောက်ပါ Plan တစ်ခုကို ရွေးချယ်ပြီး ၁ မိနစ်အတွင်း စတင်နိုင်ပါသည်။',
        'download_title': 'Key မဝယ်မီ Happ proxy app ကို အရင်ထည့်သွင်းထားရန် လိုအပ်ပါသည်',
        'download_text': 'Key ကို အသက်သွင်းမတိုင်မီ Happ Proxy App ကို Download လုပ်ပါ။ အခမဲ့ဖြစ်ပြီး ၃၀ စက္ကန့်ခန့်သာ ကြာပါသည်။',
        'btn_download': 'App Download ရယူမည်',
        'how_it_works': 'လုပ်ဆောင်ပုံ',
        'step_1': 'အောက်ပါ Plan တစ်ခုကို ရွေးချယ်ပါ',
        'step_2': 'KPay, Wave, AYA Pay ဖြင့် ငွေလွှဲပါ',
        'step_3': 'Screenshot တင်ပါ — စက္ကန့်ပိုင်းအတွင်း အတည်ပြုပေးသည်',
        'step_4': 'VPN Key အသုံးပြုနိုင်ပါပြီ!',

        // ── Plans ──
        'loading_plans': 'Plan များ ရှာဖွေနေပါသည်...',
        'title_extend': 'Key သက်တမ်းတိုးရန်',
        'title_choose_plan': 'Plan ရွေးချယ်ရန်',
        'title_top_up': 'Wallet ငွေဖြည့်ရန်',
        'top_up_amount': '{{amount}} {{currency}}',
        'subtitle_extending': 'သက်တမ်းတိုးမည့် Key: {{label}}',
        'subtitle_new_key': 'Plan တစ်ခုကို ရွေးချယ်ပါ — နှိပ်ပြီး ဆက်လက်ဆောင်ရွက်ပါ',
        'subtitle_new_key_hint': 'VPN Key အသစ်တစ်ခု ရရှိပါမည်',
        'best_value': '✦ အသင့်တော်ဆုံး',
        'unlimited': 'Data ကန့်သတ်မှု မရှိ',
        'per_day': 'တစ်ရက်လျင် {{currency}} ကျပ်',
        'new_expiry': '✓ သက်တမ်းကုန်မည့် ရက်: {{date}}',

        // ── Plans Help ──
        'help_extend_info': '✅ သက်တမ်းတိုးခြင်းသည် လက်ကျန် ရက်နှင့် Data ပေါ်တွင် ထပ်ပေါင်းထည့်ပေးသည် — ဆုံးရှုံးမှု မရှိပါ၊ Key နှင့် Setting များ အပြောင်းအလဲ မရှိပါ။',
        'help_new_key_info': 'Plan တစ်ခုဝယ်တိုင်း VPN Key အသစ်တစ်ခု ရရှိပါမည်။ Unlimited Plan တွင် Data ကန့်သတ်ချက် မရှိပါ။ Limited Plan သည် ဈေးနှုန်းသက်သာသော်လည်း လစဉ် Data ကန့်သတ်ချက် ရှိပါသည်။',
        'help_filtered_plans': '{{type}} Plan များကိုသာ ဖော်ပြထားပါသည် — သင်၏ {{type_desc}} Key အမျိုးအစားနှင့် ကိုက်ညီသောကြောင့် ဖြစ်သည်။',
        'help_payments': '💳 လက်ခံသော ငွေပေးချေမှုများ: KPay · Wave · AYA Pay — Plan ရွေးပြီးပါက ငွေလွှဲ၍ Screenshot တင်ကာ ချက်ချင်း အတည်ပြုပေးပါသည်။',

        // ── Checkout ──
        'creating_purchase': 'အော်ဒါ ပြင်ဆင်နေပါသည်...',
        'payment_details': 'ငွေပေးချေမှု အသေးစိတ်',
        'amount_to_send': 'လွှဲရမည့် ငွေပမာဏ (အတိအကျ)',
        'send_to_phone': 'ဤ ဖုန်းနံပါတ်သို့ လွှဲပေးပါ',
        'warning_no_note': '⚠️ မှတ်ချက် (Remark) တွင် "Payment" ဟုသာ ရေးပါ — အခြားဘာမှ မရေးပါနှင့်',
        'upload_btn': '📤 ငွေလွှဲပြေစာ (Screenshot) တင်မည်',
        'uploading_btn': 'စစ်ဆေးနေပါသည်...',
        'upload_hint': 'ငွေလွှဲပြီးပါက အတည်ပြုချက် စာမျက်နှာ၏ Screenshot ကို ရှင်းရှင်းလင်းလင်း ရိုက်ပြီး ဤနေရာတွင် တင်ပေးပါ။ ကျွန်ုပ်တို့ Systems မှ စက္ကန့်ပိုင်းအတွင်း အလိုအလျောက် စစ်ဆေးပေးပါမည်။',

        // ── Checkout Guide Steps ──
        'guide_title': 'ငွေပေးချေနည်း',
        'guide_step_1': 'Mobile Banking App ဖွင့်ပါ',
        'guide_step_1_hint': 'KPay, Wave Money, သို့မဟုတ် AYA Pay',

        'guide_step_2': 'ဤဖုန်းနံပါတ်သို့ ငွေပမာဏအတိအကျ လွှဲပါ',
        'label_amount': 'ပမာဏ (ကျပ်)',
        'label_phone': 'ဖုန်းနံပါတ်',

        'guide_step_3': 'Remark/Note တွင် "Payment" ဟုသာ ရေးပါ',
        'guide_step_3_hint': 'VPN, Wavy, Outline — ဘာမှ လုံးဝ မရေးပါနှင့်',

        'guide_step_4': 'ငွေလွှဲပြေစာ Screenshot ရိုက်ပြီး အောက်တွင်တင်ပါ',
        'guide_step_4_hint': 'စက္ကန့်ပိုင်းအတွင်း အလိုအလျောက် အတည်ပြုပေးသည်',

        'tap_to_copy': 'နှိပ်ပြီး ကူးယူပါ',
        'copied': 'ကူးယူပြီး ✓',
        'important_warning': '⚠️ အထူးသတိပြုရန်: ပြထားသော ပမာဏ အတိအကျကိုသာ လွှဲပါ။ Remark/Note တွင် "Payment" ဟုသာ ရေးပါ။ အခြားစာသား (VPN, Wavy, Outline) ရေးမိပါက စစ်ဆေးမှု မအောင်မြင်ဘဲ Key ရရှိမည် မဟုတ်ပါ။',

        // ── Success / Error ──
        'success_title': '✅ ငွေပေးချေမှု အောင်မြင်ပါသည်!',
        'success_extend': 'Key သက်တမ်းတိုးခြင်း အောင်မြင်ပါပြီ — ရက်နှင့် Data များ ထပ်ပေါင်းလိုက်ပါပြီ၊ အဆင်သင့်ဖြစ်ပါပြီ။',
        'success_new': 'သင်၏ VPN Key အသစ် အသင့်ဖြစ်ပါပြီ။ Happ တွင် ထည့်ရန် အောက်ပါကို နှိပ်ပါ။',
        'success_tip_extend': 'ပြန်သွားပြီး Key ၏ သက်တမ်းရက်အသစ်နှင့် ကျန်ရှိ Data ကို စစ်ဆေးနိုင်ပါသည်။',
        'success_tip_new': 'အောက်ပါ "Happ Proxy တွင် ဖွင့်မည်" ကို နှိပ်ပြီး VPN ချက်ချင်းစတင်နိုင်ပါသည်၊ သို့မဟုတ် Key Link ကို အခြား VPN App တွင် သုံးနိုင်ပါသည်။',
        'verify_error_tip': '💡 ဖြေရှင်းနည်း: ပမာဏနှင့် ဖုန်းနံပါတ် တိကျမှုရှိ/မရှိ စစ်ပါ၊ Screenshot တွင် Transaction အတည်ပြုချက် (Balance screen မဟုတ်ဘဲ) ထင်ရှားမြင်နိုင်မှု ရှိ/မရှိ စစ်ဆေးပါ။',

        // ── Wallet ──
        'wallet_title': 'Wavy Wallet',
        'wallet_subtitle': 'လက်ကျန်ငွေ နှင့် အလိုအလျောက် သက်တမ်းတိုးစနစ်',
        'current_balance': 'လက်ကျန် ငွေပမာဏ',
        'top_up_wallet': '+ ငွေဖြည့်မည်',
        'auto_renew_title': 'အလိုအလျောက် သက်တမ်းတိုး',
        'auto_renew_enabled': 'ဖွင့်ထားသည် — Key သက်တမ်းကုန်ပါက အလိုအလျောက် တိုးပေးမည်',
        'auto_renew_disabled': 'ပိတ်ထားသည် — ဖွင့်ရန် နှိပ်ပါ',
        'transaction_history': 'ငွေစာရင်း မှတ်တမ်း',
        'no_transactions': 'မှတ်တမ်း မရှိသေးပါ',
        'transaction_topup': 'Wallet ငွေဖြည့်ခြင်း',
        'transaction_purchase': 'Plan ဝယ်ယူခြင်း',
        'transaction_refund': 'ငွေပြန်အမ်းခြင်း',
        'wallet_info': 'Wallet အတွင်းရှိ ငွေများသည် သက်တမ်းကုန်ဆုံးခြင်း မရှိဘဲ မည်သည့် Plan အတွက်မဆို အသုံးပြုနိုင်ပါသည်။',
        'no_refund_title': 'ငွေပြန်အမ်းနိုင်မှု မူဝါဒ',
        'no_refund_desc': 'Plan ဝယ်ယူပြီးပါက ငွေပြန်အမ်းခြင်း ပြုလုပ်၍ မရပါ။ Wallet ငွေများသည်မူ သက်တမ်းကုန်ဆုံးခြင်း မရှိပါ။',

        // ── Referral ──
        'referral_earnings': '🤝 မိတ်ဆက်ကြေး',
        'friends_invited': 'ဖိတ်ခေါ်ထားသော သူငယ်ချင်းများ:',
        'total_earned': 'စုစုပေါင်း ရရှိငွေ:',
        'share_link': 'Link မျှဝေမည် →',
        'referral_pending': 'စောင့်ဆိုင်းဆဲ',
        'referral_bonus_received': 'ဘောနပ်စ် ရရှိပါသည်',
        'referral_wallet_chip': 'မိတ်ဆက် {{count}} ယောက် · {{earned}} ကျပ် ရရှိပါသည် →',
        'referral_checkout_title': '🎁 သူငယ်ချင်းများကို ဖိတ်ခေါ်ပါ',
        'referral_checkout_desc': 'သူငယ်ချင်းများကို Wavy သို့ ဖိတ်ခေါ်ပြီး နှစ်ဦးစလုံး အခမဲ့ VPN list ရယူပါ!',
        'referral_checkout_btn': 'Link မျှဝေမည်',

        // ── Wallet Tips ──
        'wallet_tips_title': 'Wallet ၏ အကျိုးကျေးဇူးများ',
        'wallet_tip_1_title': '၂၄ နာရီ ဝန်ဆောင်မှု မပြတ်တောက်ပါ',
        'wallet_tip_1_desc': 'Auto-renewal ဖွင့်ထားပါ — VPN သည် သက်တမ်းကုန်သည့်အခါ အလိုအလျောက် သက်တမ်းတိုးပေးမည်၊ ပြတ်တောက်ခြင်း မရှိပါ။',
        'wallet_tip_2_title': 'Key ချက်ချင်း ရရှိမည်',
        'wallet_tip_2_desc': 'Screenshot တင်ပြီး စောင့်ဆိုင်းရန် မလိုပါ — Wallet ငွေဖြင့် ဝယ်ယူပါက Key ကို စက္ကန့်ပိုင်းအတွင်း ရရှိမည်။',
        'wallet_tip_3_title': 'တစ်ကြိမ်ငွေဖြည့်ပြီး စိတ်ချလက်ချ သုံးနိုင်သည်',
        'wallet_tip_3_desc': 'ငွေခဏခဏလွှဲစရာမလိုဘဲ အလိုအလျောက်သက်တမ်းတိုးစနစ်ဖြင့် အချိန်အကြာကြီး အသုံးပြုနိုင်ပါမည်။',

        // ── Wallet Payment ──
        'pay_with_wallet': '⚡ Wallet ဖြင့် ငွေပေးချေမည်',
        'your_balance': 'သင်၏ လက်ကျန်ငွေ:',
        'or_pay_manually': 'သို့မဟုတ် Mobile Banking ဖြင့် ကိုယ်တိုင် ငွေလွှဲပါ:',
        'accepted_methods': 'KPay · Wave · AYA Pay',
        'wallet_pay_success': 'Wallet ဖြင့် ပေးချေပြီးပါပြီ',
        'check_home_for_key': 'Key အသက်ဝင်ပါပြီ — \"ကျွန်ုပ်၏ Key များ\" တွင် ကြည့်ရှုနိုင်ပါသည်။',
        'funds_added': 'Wallet ထဲသို့ ငွေဖြည့်သွင်း အောင်မြင်ပါပြီ။',
        'wallet_pay_processing': 'ဆောင်ရွက်နေပါသည်...',
        'wallet_pay_btn': 'Wallet မှ {{amount}} {{currency}} ပေးချေမည်',
        'wallet_pay_error': 'Wallet ငွေပေးချေမှု မအောင်မြင်ပါ။ လက်ကျန်ငွေ စစ်ဆေးပြီး ထပ်ကြိုးစားပါ။',

        // ── Wallet Top-up Success ──
        'success_topup_desc': 'Wallet ငွေဖြည့်သွင်းမှု အောင်မြင်ပါပြီ။',
        'back_to_wallet': 'Wallet သို့ ပြန်သွားမည်',

        // ── Wallet Empty States ──
        'wallet_empty_title': 'မှတ်တမ်း မရှိသေးပါ',
        'wallet_empty_desc': 'ပထမဆုံး ငွေဖြည့်မှု သို့မဟုတ် ဝယ်ယူမှုပြီးနောက် မှတ်တမ်းများ ဤနေရာတွင် ပေါ်လာပါမည်။',

        // ── Promo Code ──
        'promo_placeholder': 'Promo code ထည့်ပါ',
        'promo_apply': 'သုံးမည်',
        'promo_validating': 'စစ်ဆေးနေပါသည်...',
        'promo_valid': '🎉 Code သုံးပြီး — {{percent}}% လျှော့ဈေးရပါပြီ!',
        'promo_invalid': '❌ Code မမှန်ကန်ပါ သို့မဟုတ် သက်တမ်းကုန်ဆုံးပြီ',
    }
};
