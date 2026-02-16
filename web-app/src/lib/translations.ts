export type Language = 'en' | 'my';

export const translations: Record<Language, Record<string, string>> = {
    en: {
        // General
        'loading': 'Loading...',
        'retry': 'Retry',
        'error_prefix': 'Error: ',
        'powered_by': 'Powered by Remnawave',
        'open_in_tg': 'Open this app inside Telegram',

        // Navigation / Steps
        'nav_plan': 'Plan',
        'nav_payment': 'Payment',
        'nav_verify': 'Verify',
        'back_button': 'Back',
        'back_to_plans': 'Back to Plans',
        'go_home': 'Go to My Keys',

        // Home
        'home_title': 'Remnawave VPN',
        'active_key_count': '{{count}} active key',
        'active_key_count_plural': '{{count}} active keys',
        'no_active_keys': 'No active keys',
        'key_active': '● Active',
        'key_expired': '○ Expired',
        'days_left': 'days left',
        'expires_on': 'Expires {{date}}',
        'data_usage': 'Data Usage',
        'btn_add_happ': '🚀 Add to Happ',
        'btn_extend': '⏳ Extend',
        'btn_buy_new': '➕ Buy New Key',
        'btn_get_started': '🚀 Get Started — Buy a Plan',

        // Home Help
        'help_expired': 'This key has expired. Tap Extend below to add more days and keep using it.',
        'help_btn_add': 'Add to Happ — auto-imports this key into the Happ VPN app',
        'help_btn_extend': 'Extend — add more days & data to this key',
        'help_btn_copy': 'copy link to use in any VPN app',
        'tip_multi_key': 'Tip: You can have multiple VPN keys for different devices, or extend an existing one to add more days.',

        // Welcome
        'welcome_title': 'Welcome!',
        'welcome_text': 'You don\'t have any VPN keys yet. Buy a plan to get started — it only takes a minute!',
        'how_it_works': 'How it works',
        'step_1': 'Choose a plan below',
        'step_2': 'Pay via mobile banking',
        'step_3': 'Upload a screenshot — we verify it instantly',
        'step_4': 'Your VPN key is ready to use!',

        // Plans
        'loading_plans': 'Loading plans...',
        'title_extend': '⏳ Extend Key',
        'title_choose_plan': '💎 Choose a Plan',
        'subtitle_extending': 'Extending: {{label}}',
        'subtitle_new_key': 'Pick a plan that fits your needs — tap to continue',
        'subtitle_new_key_hint': 'A new subscription key will be created',
        'best_value': 'Best Value',
        'unlimited': '♾️ Unlimited',
        'per_day': '/{{currency}}/day',
        'new_expiry': '✓ New expiry: {{date}}',

        // Plans Help
        'help_extend_info': 'Extending adds more days and data to your existing key. Your current remaining days and data are kept — nothing is lost!',
        'help_new_key_info': 'Each plan gives you a new VPN key with a set number of days and data. Unlimited plans have no data cap. Limited plans are cheaper but have a monthly data limit.',
        'help_filtered_plans': 'Only {{type}} plans are shown because your key is {{type_desc}} plan.',
        'help_payments': 'Accepted payments: KPay · Wave · AYA Pay. After selecting a plan, you\'ll send money and upload a screenshot for instant verification.',

        // Checkout
        'creating_purchase': 'Creating purchase...',
        'payment_details': '💳 Payment Details',
        'amount_to_send': 'Amount to Send',
        'send_to_phone': 'Send to Phone Number',
        'warning_no_note': '⚠️ Do NOT add any note or remark',
        'upload_btn': '📸 Upload Payment Screenshot',
        'uploading_btn': '🔍 Verifying your payment...',
        'upload_hint': 'After you\'ve sent the money, take a screenshot of the confirmation and upload it here. We\'ll verify it automatically in seconds.',

        // Checkout Guide
        'guide_title': '📝 Follow these steps',
        'guide_step_1': 'Open your banking app',
        'guide_step_1_hint': 'KPay, Wave, or AYA Pay',
        'guide_step_2': 'Send this exact amount',
        'guide_step_3': 'To this phone number',
        'tap_to_copy': '📋 Tap to copy',
        'copied': '✅ Copied',
        'guide_step_4': 'Leave the note/remark empty',
        'guide_step_4_hint': 'Do NOT write anything like "VPN" or "subscription"',
        'important_warning': 'Important: Send the exact amount shown above. Different amounts will fail verification. Do not add any notes or remarks.',

        // Success / Fail
        'success_title': 'Payment Verified!',
        'success_extend': 'Your key has been extended successfully! Extra days and data have been added.',
        'success_new': 'Your new VPN key has been created and is ready to use.',
        'success_tip_extend': 'Go back to see your updated key with the new expiry date.',
        'success_tip_new': 'Go back to your keys and tap "Add to Happ" to start using your VPN, or copy the link for any VPN app.',
        'verify_error_tip': 'Double-check that the amount and phone number match exactly. Make sure the screenshot clearly shows the transaction details.',
    },
    my: {
        // General
        'loading': 'ခေတ္တစောင့်ပါ...',
        'retry': 'ထပ်ကြိုးစားမည်',
        'error_prefix': 'Error: ',
        'powered_by': 'Remnawave မှ ပံ့ပိုးသည်',
        'open_in_tg': 'Telegram အတွင်း ဖွင့်ပါ',

        // Navigation / Steps
        'nav_plan': 'အစီအစဉ်',
        'nav_payment': 'ငွေပေးချေမှု',
        'nav_verify': 'စစ်ဆေးခြင်း',
        'back_button': 'နောက်သို့',
        'back_to_plans': 'အစီအစဉ်များသို့',
        'go_home': 'ကျွန်ုပ်၏ Keys များသို့',

        // Home
        'home_title': 'Remnawave VPN',
        'active_key_count': 'အသက်ဝင်နေသော Key ၁ ခု',
        'active_key_count_plural': 'အသက်ဝင်နေသော Key {{count}} ခု',
        'no_active_keys': 'အသက်ဝင်သော Key မရှိပါ',
        'key_active': '● Active',
        'key_expired': '○ သက်တမ်းကုန်',
        'days_left': 'ရက်ကျန်',
        'expires_on': 'သက်တမ်းကုန်မည့်ရက် {{date}}',
        'data_usage': 'Data သုံးစွဲမှု',
        'btn_add_happ': '🚀 Happ သို့ ထည့်မည်',
        'btn_extend': '⏳ သက်တမ်းတိုး',
        'btn_buy_new': '➕ Key အသစ်၀ယ်မည်',
        'btn_get_started': '🚀 စတင်လိုက်ပါ — အစီအစဉ် ရွေးမည်',

        // Home Help
        'help_expired': 'Key သက်တမ်းကုန်သွားပါပြီ။ ဆက်လက်အသုံးပြုရန် အောက်ပါ သက်တမ်းတိုး ခလုတ်ကို နှိပ်ပါ။',
        'help_btn_add': 'Happ သို့ ထည့်မည် — Happ VPN app ထဲသို့ အလိုအလျောက် ထည့်သွင်းပေးမည်',
        'help_btn_extend': 'သက်တမ်းတိုး — ရက်နှင့် Data ပမာဏ ထပ်ပေါင်းထည့်မည်',
        'help_btn_copy': 'VPN app တခုခုတွင်သုံးရန် link ကူးယူမည်',
        'tip_multi_key': 'အကြံပြုချက်: ဖုန်းအမျိုးမျိုးအတွက် Key အများကြီး ဝယ်ထားနိုင်သလို၊ ရှိပြီးသား Key ကိုလည်း သက်တမ်းတိုးနိုင်ပါသည်။',

        // Welcome
        'welcome_title': 'မင်္ဂလာပါ!',
        'welcome_text': 'သင့်တွင် VPN key မရှိသေးပါ။ စတင်အသုံးပြုရန် အစီအစဉ် တစ်ခုကို ဝယ်ယူပါ — ၁ မိနစ်သာ ကြာပါမည်!',
        'how_it_works': 'အလုပ်လုပ်ပုံ',
        'step_1': 'အောက်တွင် အစီအစဉ် တစ်ခု ရွေးပါ',
        'step_2': 'Mobile Banking ဖြင့် ငွေလွှဲပါ',
        'step_3': 'ငွေလွှဲပြေစာ (Screenshot) တင်ပါ — ချက်ချင်း စစ်ဆေးပေးသည်',
        'step_4': 'VPN key စတင် အသုံးပြုနိုင်ပါပြီ!',

        // Plans
        'loading_plans': 'အစီအစဉ်များ ရယူနေသည်...',
        'title_extend': '⏳ သက်တမ်းတိုးခြင်း',
        'title_choose_plan': '💎 အစီအစဉ် ရွေးချယ်ပါ',
        'subtitle_extending': 'သက်တမ်းတိုးနေသည်: {{label}}',
        'subtitle_new_key': 'သင့်လိုအပ်ချက်နှင့် ကိုက်ညီမည့် အစီအစဉ်ကို ရွေးပါ',
        'subtitle_new_key_hint': 'Subscription key အသစ် ရရှိပါမည်',
        'best_value': 'တန်ဖိုးအရှိဆုံး',
        'unlimited': '♾️ အကန့်အသတ်မဲ့',
        'per_day': '/{{currency}}/ရက်',
        'new_expiry': '✓ သက်တမ်းကုန်မည့်ရက်သစ်: {{date}}',

        // Plans Help
        'help_extend_info': 'သက်တမ်းတိုးခြင်းသည် ရှိပြီးသား Key တွင် ရက်နှင့် Data ပမာဏကို ပေါင်းထည့်ပေးသည်။ လက်ကျန် ရက်နှင့် Data များ ဆုံးရှုံးမည် မဟုတ်ပါ!',
        'help_new_key_info': 'အစီအစဉ်သစ် ဝယ်ယူတိုင်း VPN key အသစ်တစ်ခု ရရှိပါမည်။ Unlimited အစီအစဉ်များတွင် Data ကန့်သတ်ချက် မရှိပါ။ Limited အစီအစဉ်များသည် ဈေးသက်သာသော်လည်း Data ကန့်သတ်ချက် ရှိပါသည်။',
        'help_filtered_plans': 'သင်၏ Key သည် {{type_desc}} ဖြစ်သောကြောင့် {{type}} အစီအစဉ်များကိုသာ ပြထားပါသည်။',
        'help_payments': 'လက်ခံသော ငွေပေးချေမှုများ: KPay · Wave · AYA Pay။ အစီအစဉ် ရွေးပြီးပါက ငွေလွှဲပြီး Screenshot တင်ကာ အသုံးပြုနိုင်ပါသည်။',

        // Checkout
        'creating_purchase': 'အော်ဒါ တင်နေသည်...',
        'payment_details': '💳 ငွေပေးချေမှု အချက်အလက်',
        'amount_to_send': 'လွှဲရမည့် ပမာဏ',
        'send_to_phone': 'ငွေလက်ခံမည့် ဖုန်းနံပါတ်',
        'warning_no_note': '⚠️ မှတ်ချက် (Remark) တွင် ဘာမှ မရေးပါနှင့်',
        'upload_btn': '📸 ငွေလွှဲပြေစာ တင်မည်',
        'uploading_btn': '🔍 စစ်ဆေးနေသည်...',
        'upload_hint': 'ငွေလွှဲပြီးပါက Transaction ပြေစာ Screenshot ကို ရိုက်ပြီး ဤနေရာတွင် တင်ပေးပါ။ စက္ကန့်ပိုင်းအတွင်း အလိုအလျောက် စစ်ဆေးပေးပါမည်။',

        // Checkout Guide
        'guide_title': '📝 ဤအဆင့်များအတိုင်း လုပ်ဆောင်ပါ',
        'guide_step_1': 'သင့် Banking App ဖွင့်ပါ',
        'guide_step_1_hint': 'KPay, Wave, သို့မဟုတ် AYA Pay',
        'guide_step_2': 'ဤပမာဏ အတိအကျ လွှဲပါ',
        'guide_step_3': 'ဤဖုန်းနံပါတ်သို့ လွှဲပါ',
        'tap_to_copy': '📋 ဖိပီး ကူးယူပါ',
        'copied': '✅ ကူးယူပြီး',
        'guide_step_4': 'Note/Remark တွင် ဘာမှ မရေးပါနှင့်',
        "guide_step_4_hint": '"VPN" သို့မဟုတ် "Subscription" စာသားများ လုံးဝ မရေးပါနှင့်',
        'important_warning': 'အရေးကြီးသည်: ပြထားသော ပမာဏ အတိအကျ လွှဲပါ။ ပမာဏ မှားယွင်းပါက စစ်ဆေး၍ ရမည်မဟုတ်ပါ။ Note/Remark တွင် ဘာမှ မရေးပါနှင့်။',

        // Success / Fail
        'success_title': 'ငွေပေးချေမှု အောင်မြင်သည်!',
        'success_extend': 'သက်တမ်းတိုးခြင်း အောင်မြင်ပါသည်။ ရက်နှင့် Data များ ပေါင်းထည့်လိုက်ပါပြီ။',
        'success_new': 'VPN key အသစ် ရရှိပါပြီ။ စတင် အသုံးပြုနိုင်ပါပြီ။',
        'success_tip_extend': 'နောက်သို့ ပြန်သွားပြီး သက်တမ်းရက် အသစ်ကို စစ်ဆေးနိုင်ပါသည်။',
        'success_tip_new': 'နောက်သို့ ပြန်သွားပြီး "Happ သို့ ထည့်မည်" ကို နှိပ်၍ အသုံးပြုနိုင်ပါပြီ။',
        'verify_error_tip': 'ပမာဏနှင့် ဖုန်းနံပါတ် တူညီမှု ရှိမရှိ ပြန်စစ်ပါ။ Screenshot တွင် Transaction အချက်အလက်များ ထင်ရှားမှု ရှိမရှိ စစ်ဆေးပါ။',
    }
};
