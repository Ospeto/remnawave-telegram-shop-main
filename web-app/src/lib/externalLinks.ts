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

export function resolveSupportUrl(supportUrl?: string | null, botUrl?: string | null): string | null {
    return sanitizeExternalUrl(supportUrl) ?? sanitizeExternalUrl(botUrl);
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
