import { describe, expect, it } from 'vitest';

import { APIError } from './http';
import { apiErrorCode, localizedAPIError, parseAPIErrorBody } from './apiErrors';

const t = (key: string, params?: Record<string, string | number>) => {
    if (key === 'error_code_purchase_not_found') return 'Purchase not found';
    if (key === 'error_code_unauthorized') return 'Your session has expired';
    if (key === 'error_code_generic') return params ? `generic ${JSON.stringify(params)}` : 'Something went wrong';
    return key;
};

describe('parseAPIErrorBody', () => {
    it('reads the code and message from the structured error body', () => {
        const body = parseAPIErrorBody('{"code":"purchase_not_found","message":"Purchase not found"}');

        expect(body.code).toBe('purchase_not_found');
        expect(body.message).toBe('Purchase not found');
    });

    it('keeps a legacy plain-text body usable as the message', () => {
        const body = parseAPIErrorBody('Purchase is not awaiting verification');

        expect(body.code).toBeUndefined();
        expect(body.message).toBe('Purchase is not awaiting verification');
    });

    it('treats an empty body as no information', () => {
        expect(parseAPIErrorBody('')).toEqual({});
        expect(parseAPIErrorBody(null)).toEqual({});
    });
});

describe('localizedAPIError', () => {
    // U5: the Mini App used to print the server's English sentence under a
    // Burmese UI. A known code now resolves through the translation catalogue.
    it('maps a known code to localized copy instead of the English sentence', () => {
        const error = new APIError(404, '{"code":"purchase_not_found","message":"Purchase not found"}');

        expect(localizedAPIError(t, error, 'error_code_generic')).toBe('Purchase not found');
        expect(apiErrorCode(error)).toBe('purchase_not_found');
    });

    it('falls back to the server message for a code this build does not know', () => {
        const error = new APIError(500, '{"code":"brand_new_code","message":"Disk on fire"}');

        expect(localizedAPIError(t, error, 'error_code_generic')).toBe('Disk on fire');
    });

    it('falls back to the caller key when the body carries nothing', () => {
        expect(localizedAPIError(t, new APIError(500, ''), 'error_code_generic')).toBe('Something went wrong');
        expect(localizedAPIError(t, new Error('boom'), 'error_code_generic')).toBe('boom');
        expect(localizedAPIError(t, undefined, 'error_code_generic')).toBe('Something went wrong');
    });

    it('uses localized copy when the catalogue has no entry for the code', () => {
        const error = new APIError(401, '{"code":"unauthorized"}');

        expect(localizedAPIError(t, error, 'error_code_generic')).toBe('Your session has expired');
    });
});
