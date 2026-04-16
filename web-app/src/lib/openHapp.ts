/**
 * Opens a Happ deep-link URL using dual strategy:
 * 1. Hidden iframe (works on most Android/iOS)
 * 2. tg.openLink with the server-backed /redirect fallback page
 */
export function openHappLink(
    happUrl: string,
    redirectUrl: string | undefined,
    tg: { openLink: (url: string) => void } | null
) {
    // Strategy 1: Hidden iframe
    const iframe = document.createElement('iframe');
    iframe.style.display = 'none';
    iframe.src = happUrl;
    document.body.appendChild(iframe);
    setTimeout(() => iframe.remove(), 3000);

    // Strategy 2: tg.openLink with redirect fallback
    const fallbackUrl = redirectUrl ? new URL(redirectUrl, window.location.origin).toString() : happUrl;
    if (tg?.openLink) {
        tg.openLink(fallbackUrl);
    } else {
        window.open(fallbackUrl, '_blank', 'noopener,noreferrer');
    }
}
