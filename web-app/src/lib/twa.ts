// Safe synchronous access to Telegram WebApp
// The SDK script in <head> loads synchronously, so window.Telegram.WebApp
// is available immediately when React renders.

export function useTelegram() {
    const tg = window.Telegram?.WebApp || null;

    // Call ready() and expand() once
    if (tg && !(tg as any).__readyCalled) {
        tg.ready();
        tg.expand();
        (tg as any).__readyCalled = true;
    }

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
        colorScheme: tg?.colorScheme || 'light',
        themeParams: tg?.themeParams,
    };
}
