import { afterEach, describe, expect, it, vi } from 'vitest';
import { openHappLink } from './openHapp';

describe('openHappLink', () => {
    afterEach(() => {
        vi.restoreAllMocks();
        document.body.innerHTML = '';
    });

    it('opens the server redirect page through Telegram when available', () => {
        const tg = { openLink: vi.fn() };
        openHappLink('happ://add/https://example.com/sub', tg);

        expect(tg.openLink).toHaveBeenCalledWith(
            `${window.location.origin}/redirect?url=${encodeURIComponent('happ://add/https://example.com/sub')}`
        );

        const iframe = document.querySelector('iframe');
        expect(iframe?.getAttribute('src')).toBe('happ://add/https://example.com/sub');
    });

    it('falls back to window.open when Telegram is unavailable', () => {
        const windowOpen = vi.spyOn(window, 'open').mockImplementation(() => null);

        openHappLink('happ://add/https://example.com/sub', null);

        expect(windowOpen).toHaveBeenCalledWith(
            `${window.location.origin}/redirect?url=${encodeURIComponent('happ://add/https://example.com/sub')}`,
            '_blank'
        );
    });
});
