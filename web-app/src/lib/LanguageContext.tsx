import { createContext, useCallback, useContext, useEffect, useState, ReactNode } from 'react';
import { translations, Language } from './translations';

interface LanguageContextType {
    language: Language;
    /**
     * Sets the language held by this app.
     *
     * `adoptFromServer` marks a value that arrived from the customer's profile
     * rather than from a tap, so it does not count as an explicit choice.
     */
    setLanguage: (lang: Language, options?: { adoptFromServer?: boolean }) => void;
    t: (key: string, params?: Record<string, string | number>) => string;
}

const LanguageContext = createContext<LanguageContextType | undefined>(undefined);

/** Where the app keeps the language it renders with. */
export const LANGUAGE_STORAGE_KEY = 'app_lang';

/**
 * Whether the customer has chosen a language in this app.
 *
 * The bot stores a language on the customer row and `/api/me` now reports it, so
 * the two surfaces can be brought into step instead of disagreeing invisibly.
 * An in-app tap is still the customer's own decision and outranks the stored
 * value; this flag is what distinguishes the two. It is separate from
 * `LANGUAGE_STORAGE_KEY` on purpose: the stored language may itself have been
 * adopted from the server, and re-adopting it must stay possible.
 */
export const LANGUAGE_CHOICE_KEY = 'app_lang_choice';

/** True when the customer has tapped the switcher in this app. */
export function hasExplicitLanguageChoice(): boolean {
    try {
        return localStorage.getItem(LANGUAGE_CHOICE_KEY) === 'true';
    } catch {
        return false;
    }
}

/**
 * Maps a language tag onto a supported language.
 *
 * Russian used to be a supported language; it is dropped, so an `ru` tag (from
 * storage or from the bot) resolves to nothing and the caller keeps its default.
 */
export function normalizeLanguage(value: unknown): Language | null {
    if (typeof value !== 'string') return null;
    const base = value.trim().toLowerCase().split(/[-_]/)[0];
    return base === 'en' || base === 'my' ? base : null;
}

export function LanguageProvider({ children }: { children: ReactNode }) {
    // Default to 'my' (Burmese) as requested
    const [language, setLanguageState] = useState<Language>(() => {
        try {
            return normalizeLanguage(localStorage.getItem(LANGUAGE_STORAGE_KEY)) ?? 'my';
        } catch {
            return 'my';
        }
    });

    useEffect(() => {
        try {
            localStorage.setItem(LANGUAGE_STORAGE_KEY, language);
        } catch {
            // Private-mode storage denial must not break rendering.
        }
        // Set HTML lang attribute
        document.documentElement.lang = language;
    }, [language]);

    const setLanguage = useCallback((lang: Language, options?: { adoptFromServer?: boolean }) => {
        if (!options?.adoptFromServer) {
            try {
                localStorage.setItem(LANGUAGE_CHOICE_KEY, 'true');
            } catch {
                // Private-mode storage denial must not break the switcher.
            }
        }
        setLanguageState(lang);
    }, []);

    const t = (key: string, params?: Record<string, string | number>): string => {
        const dict = translations[language];
        let text = dict[key] || translations['en'][key] || key;

        if (params) {
            Object.entries(params).forEach(([k, v]) => {
                text = text.replace(new RegExp(`{{${k}}}`, 'g'), String(v));
            });
        }
        return text;
    };

    return (
        <LanguageContext.Provider value={{ language, setLanguage, t }}>
            {children}
        </LanguageContext.Provider>
    );
}

export function useLanguage() {
    const context = useContext(LanguageContext);
    if (!context) {
        throw new Error('useLanguage must be used within a LanguageProvider');
    }
    return context;
}
