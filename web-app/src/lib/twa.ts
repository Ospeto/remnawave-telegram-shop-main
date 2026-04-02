import { useEffect } from 'react';

// Safe synchronous access to Telegram WebApp
// The SDK script in <head> loads synchronously, so window.Telegram.WebApp
// is available immediately when React renders.

let readyCalled = false;

function fallbackCopyText(text: string) {
    const textArea = document.createElement('textarea');
    textArea.value = text;
    textArea.style.position = 'fixed';
    textArea.style.left = '-9999px';
    document.body.appendChild(textArea);
    textArea.select();

    const copied = document.execCommand('copy');
    document.body.removeChild(textArea);
    if (!copied) {
        throw new Error('Clipboard copy failed');
    }
}

export async function copyText(text: string) {
    if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(text);
        return;
    }

    fallbackCopyText(text);
}

type TelegramWebApp = NonNullable<Window['Telegram']>['WebApp'];

export function openTelegramShareLink(tg: TelegramWebApp | null | undefined, url: string, text: string) {
    const shareUrl = `https://t.me/share/url?url=${encodeURIComponent(url)}&text=${encodeURIComponent(text)}`;

    if (tg && typeof tg.openTelegramLink === 'function') {
        tg.openTelegramLink(shareUrl);
        return;
    }
    if (tg && typeof tg.openLink === 'function') {
        tg.openLink(shareUrl);
        return;
    }

    window.open(shareUrl, '_blank', 'noopener,noreferrer');
}

export function createIdempotencyKey() {
    if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
        return crypto.randomUUID();
    }

    const randomBytes = typeof crypto !== 'undefined' && typeof crypto.getRandomValues === 'function'
        ? crypto.getRandomValues(new Uint8Array(16))
        : Uint8Array.from({ length: 16 }, () => Math.floor(Math.random() * 256));

    randomBytes[6] = (randomBytes[6] & 0x0f) | 0x40;
    randomBytes[8] = (randomBytes[8] & 0x3f) | 0x80;

    const hex = Array.from(randomBytes, (byte) => byte.toString(16).padStart(2, '0')).join('');
    return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}

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
