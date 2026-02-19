import { useEffect, createContext, useContext } from 'react';
import { useTelegram } from './twa';

const ThemeContext = createContext<'light' | 'dark'>('dark');

export function ThemeProvider({ children }: { children: React.ReactNode }) {
    const { colorScheme } = useTelegram();
    // Default to dark if undefined, but respect light if explicit
    const theme = colorScheme || 'dark';

    useEffect(() => {
        // Apply to both html and body for complete CSS variable coverage
        const els = [document.documentElement, document.body];
        els.forEach(el => {
            el.classList.remove('theme-light', 'theme-dark');
            el.classList.add(`theme-${theme}`);
        });
        // Also set data-theme attribute for any CSS selectors using it
        document.documentElement.setAttribute('data-theme', theme);
    }, [theme]);

    return (
        <ThemeContext.Provider value={theme}>
            {children}
        </ThemeContext.Provider>
    );
}

export const useTheme = () => useContext(ThemeContext);

