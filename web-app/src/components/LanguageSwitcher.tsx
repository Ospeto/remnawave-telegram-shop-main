import { useLanguage } from '../lib/LanguageContext';
import { useMXBrownSound } from '../lib/useMXBrownSound';

/**
 * The language switcher.
 *
 * It used to exist only on Home, so a customer arriving from a renewal
 * notification on Plans or Checkout had to navigate home before they could read
 * the screen in their own language. It is now one component, mounted on every
 * screen where a customer decides or pays.
 */
export function LanguageSwitcher({ style }: { style?: React.CSSProperties }) {
    const { t, language, setLanguage } = useLanguage();
    const { playClick } = useMXBrownSound();

    const nextLanguage = language === 'en' ? 'my' : 'en';

    return (
        <button
            type="button"
            onClick={() => { playClick(); setLanguage(nextLanguage); }}
            className="btn-secondary"
            aria-label={nextLanguage === 'my' ? t('switch_to_myanmar') : t('switch_to_english')}
            lang={nextLanguage}
            style={{ width: 'auto', padding: '8px 12px', fontSize: 'var(--font-body)', borderRadius: 20, ...style }}
        >
            {language === 'en' ? '🇺🇸 EN' : '🇲🇲 MY'}
        </button>
    );
}
