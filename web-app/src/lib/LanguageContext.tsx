import { createContext, useContext, useState, useEffect, ReactNode } from 'react';
import { translations, Language } from './translations';

interface LanguageContextType {
    language: Language;
    setLanguage: (lang: Language) => void;
    t: (key: string, params?: Record<string, string | number>) => string;
}

const LanguageContext = createContext<LanguageContextType | undefined>(undefined);

export function LanguageProvider({ children }: { children: ReactNode }) {
    // Default to 'my' (Burmese) as requested
    const [language, setLanguage] = useState<Language>(() => {
        const saved = localStorage.getItem('app_lang');
        return (saved === 'en' || saved === 'my') ? saved : 'my';
    });

    useEffect(() => {
        localStorage.setItem('app_lang', language);
        // Set HTML lang attribute
        document.documentElement.lang = language;
    }, [language]);

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
