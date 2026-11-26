function trimToNull(value?: string | null): string | null {
    const trimmed = value?.trim();
    return trimmed ? trimmed : null;
}

export function sanitizeExternalUrl(value?: string | null): string | null {
    const trimmed = trimToNull(value);
    if (!trimmed) return null;

    try {
        const url = new URL(trimmed);
        if (url.protocol !== 'https:' && url.protocol !== 'http:') {
            return null;
        }
        return url.toString();
    } catch {
        return null;
    }
}

/**
 * The support account, used when the shop has no SUPPORT_URL configured.
 *
 * This mirrors the bot's own fallback so /help, the paid activation guide and
 * the Mini App's contact link all resolve to the same door. Falling back to the
 * shop's bot URL was a different destination, so a customer who asked for help
 * from the webview landed somewhere the bot never points at.
 */
export const DEFAULT_SUPPORT_URL = 'https://t.me/ospeto';

export function resolveSupportUrl(supportUrl?: string | null, _botUrl?: string | null): string | null {
    return sanitizeExternalUrl(supportUrl) ?? DEFAULT_SUPPORT_URL;
}

export function buildTelegramStartUrl(botUrl?: string | null, startParam?: string | null): string | null {
    const sanitizedBotUrl = sanitizeExternalUrl(botUrl);
    const trimmedStartParam = trimToNull(startParam);
    if (!sanitizedBotUrl || !trimmedStartParam) {
        return null;
    }

    try {
        const url = new URL(sanitizedBotUrl);
        url.searchParams.set('start', trimmedStartParam);
        return url.toString();
    } catch {
        return null;
    }
}
