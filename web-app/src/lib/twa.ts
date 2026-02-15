import { useEffect, useState } from 'react';

// Safe access to Telegram WebApp
export function useTelegram() {
    const [tg, setTg] = useState<any>(null);

    useEffect(() => {
        if (window.Telegram?.WebApp) {
            const webApp = window.Telegram.WebApp;
            webApp.ready();
            webApp.expand();
            setTg(webApp);
        }
    }, []);

    const close = () => {
        tg?.close();
    };

    const openLink = (url: string) => {
        tg?.openLink(url); // Opens in external browser
    };

    return {
        tg,
        user: tg?.initDataUnsafe?.user,
        initData: tg?.initData,
        close,
        openLink,
        colorScheme: tg?.colorScheme || 'light',
        themeParams: tg?.themeParams,
    };
}
