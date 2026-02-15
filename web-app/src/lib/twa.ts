import { useEffect, useState } from 'react';

// Safe access to Telegram WebApp
export function useTelegram() {
    const [tg, setTg] = useState<any>(null);
    const [ready, setReady] = useState(false);

    useEffect(() => {
        // Give the SDK a moment to attach to window
        const init = () => {
            if (window.Telegram?.WebApp) {
                const webApp = window.Telegram.WebApp;
                webApp.ready();
                webApp.expand();
                setTg(webApp);
            }
            setReady(true);
        };

        // The SDK script might not have loaded yet on first render
        if (window.Telegram?.WebApp) {
            init();
        } else {
            // Fallback: wait a tick for the script to load
            const timer = setTimeout(init, 100);
            return () => clearTimeout(timer);
        }
    }, []);

    const close = () => {
        tg?.close();
    };

    const openLink = (url: string) => {
        tg?.openLink(url);
    };

    return {
        tg,
        ready, // true once we've checked for the SDK (whether it exists or not)
        user: tg?.initDataUnsafe?.user,
        initData: tg?.initData,
        close,
        openLink,
        colorScheme: tg?.colorScheme || 'light',
        themeParams: tg?.themeParams,
    };
}
