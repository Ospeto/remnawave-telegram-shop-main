import { useEffect, createContext, useContext } from 'react';
import { useTelegram } from './twa';

const ThemeContext = createContext<'light' | 'dark'>('dark');

export function ThemeProvider({ children }: { children: React.ReactNode }) {
    const { colorScheme } = useTelegram();
    // Default to dark if undefined, but respect light if explicit
    const theme = colorScheme || 'dark';

    useEffect(() => {
        // Remove previous theme classes
        document.body.classList.remove('theme-light', 'theme-dark');
        // Add current theme class
        document.body.classList.add(`theme-${theme}`);
    }, [theme]);

    return (
        <ThemeContext.Provider value={theme}>
            {children}
        </ThemeContext.Provider>
    );
}

export const useTheme = () => useContext(ThemeContext);
