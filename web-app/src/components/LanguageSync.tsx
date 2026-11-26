import { useEffect } from 'react';
import { hasExplicitLanguageChoice, normalizeLanguage, useLanguage } from '../lib/LanguageContext';
import { useUserProfile } from '../lib/userData';
import { useTelegram } from '../lib/twa';

/**
 * Brings the Mini App's language in step with the customer's stored choice.
 *
 * `/api/me` reports the language on the customer row, which is the same value
 * the bot reads before it writes a reply. Without this, the webview rendered
 * whatever happened to be in `localStorage` and the two surfaces could disagree
 * with nobody able to see it.
 *
 * An in-app choice outranks the stored value: once the customer taps the
 * switcher, this stops overriding them. The profile request itself is the shared
 * one, so this costs no extra round trip on a screen that loads `/api/me` too.
 */
export function LanguageSync() {
    const { initData, tg } = useTelegram();
    const { setLanguage, language } = useLanguage();
    const userProfile = useUserProfile();

    useEffect(() => {
        if (!initData) return;
        if (hasExplicitLanguageChoice()) return;

        let cancelled = false;
        void userProfile
            .load(initData, tg?.initDataUnsafe?.user?.id)
            .then((data) => {
                if (cancelled) return;
                const stored = normalizeLanguage(data?.user?.language);
                if (stored && stored !== language) {
                    setLanguage(stored, { adoptFromServer: true });
                }
            })
            .catch(() => {
                // A profile that cannot be read leaves the local choice alone.
            });

        return () => {
            cancelled = true;
        };
    }, [initData, language, setLanguage, tg, userProfile]);

    return null;
}
