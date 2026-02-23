import { useEffect } from 'react';

// Safe synchronous access to Telegram WebApp
// The SDK script in <head> loads synchronously, so window.Telegram.WebApp
// is available immediately when React renders.

let readyCalled = false;

export function useTelegram() {
    const tg = window.Telegram?.WebApp || null;

    useEffect(() => {
        // Call ready() and expand() once
        if (tg && !readyCalled) {
            tg.ready();
            tg.expand();
            readyCalled = true;
        }
    }, [tg]);

    const close = () => {
        tg?.close();
    };

    const openLink = (url: string) => {
        tg?.openLink(url);
    };

    return {
        tg,
        user: tg?.initDataUnsafe?.user,
        initData: tg?.initData || '',
        close,
        openLink,
        colorScheme: tg?.colorScheme || 'dark',
        themeParams: tg?.themeParams,
    };
}
