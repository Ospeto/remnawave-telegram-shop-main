import { afterEach, describe, expect, it, vi } from 'vitest';
import { openHappLink } from './openHapp';

describe('openHappLink', () => {
    afterEach(() => {
        vi.restoreAllMocks();
        document.body.innerHTML = '';
    });

    it('opens the server redirect page through Telegram when available', () => {
        const tg = { openLink: vi.fn() };
        openHappLink('happ://add/https://example.com/sub', '/redirect?token=signed-token', tg);

        expect(tg.openLink).toHaveBeenCalledWith(
            `${window.location.origin}/redirect?token=signed-token`
        );

        const iframe = document.querySelector('iframe');
        expect(iframe?.getAttribute('src')).toBe('happ://add/https://example.com/sub');
    });

    it('falls back to window.open when Telegram is unavailable', () => {
        const windowOpen = vi.spyOn(window, 'open').mockImplementation(() => null);

        openHappLink('happ://add/https://example.com/sub', '/redirect?token=signed-token', null);

        expect(windowOpen).toHaveBeenCalledWith(
            `${window.location.origin}/redirect?token=signed-token`,
            '_blank',
            'noopener,noreferrer'
        );
    });
});
