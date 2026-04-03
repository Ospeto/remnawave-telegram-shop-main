/**
 * Opens a Happ deep-link URL using dual strategy:
 * 1. Hidden iframe (works on most Android/iOS)
 * 2. tg.openLink with the server-backed /redirect fallback page
 */
export function openHappLink(
    happUrl: string,
    tg: { openLink: (url: string) => void } | null
) {
    // Strategy 1: Hidden iframe
    const iframe = document.createElement('iframe');
    iframe.style.display = 'none';
    iframe.src = happUrl;
    document.body.appendChild(iframe);
    setTimeout(() => iframe.remove(), 3000);

    // Strategy 2: tg.openLink with redirect fallback
    const redirectUrl = `${window.location.origin}/redirect?url=${encodeURIComponent(happUrl)}`;
    if (tg?.openLink) {
        tg.openLink(redirectUrl);
    } else {
        window.open(redirectUrl, '_blank');
    }
}
