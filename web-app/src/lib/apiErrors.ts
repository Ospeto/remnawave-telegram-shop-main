import { APIError } from './http';

/**
 * Machine-readable API error shape.
 *
 * U5: the server used to answer with `http.Error` plain text such as
 * "Purchase is not awaiting verification", which the Mini App rendered
 * verbatim under a Burmese UI. Every user-facing failure now carries a stable
 * `code`, and the client turns that code into localized copy; the English
 * sentence is only a fallback for codes this build does not know yet.
 */
export interface APIErrorBody {
    code?: string;
    message?: string;
    /** Present on the 409 conflict that reports an unfinished payment. */
    pending_purchase?: unknown;
}

const CODE_TRANSLATION_KEYS: Record<string, string> = {
    unauthorized: 'error_code_unauthorized',
    session_expired: 'error_code_session_expired',
    user_not_found: 'error_code_user_not_found',
    database_error: 'error_code_database',
    internal_error: 'error_code_internal',
    service_unavailable: 'error_code_service_unavailable',
    method_not_allowed: 'error_code_generic',
    invalid_request_body: 'error_code_invalid_request_body',
    missing_purchase_id: 'error_code_generic',
    invalid_purchase_id: 'error_code_generic',
    purchase_not_found: 'error_code_purchase_not_found',
    purchase_not_allowed: 'error_code_purchase_not_allowed',
    purchase_not_cancellable: 'error_code_purchase_not_cancellable',
    pending_screenshot_payment: 'error_code_pending_screenshot_payment',
    verification_rate_limited: 'error_code_verification_rate_limited',
    verification_in_progress: 'error_code_verification_in_progress',
    verification_unavailable: 'error_code_verification_unavailable',
    upload_too_large: 'error_code_upload_too_large',
    upload_invalid_file: 'error_code_upload_invalid_file',
    upload_read_failed: 'error_code_upload_invalid_file',
    upload_unsupported_type: 'error_code_upload_unsupported_type',
    trial_already_used: 'error_code_trial_already_used',
    trial_unavailable: 'error_code_trial_unavailable',
    trial_existing_panel_user: 'error_code_trial_existing_panel_user',
    trial_failed: 'error_code_trial_failed',
    promo_invalid: 'error_code_promo_invalid',
    promo_unavailable: 'error_code_promo_unavailable',
    invalid_key_id: 'error_code_invalid_key_id',
    missing_key_id: 'error_code_invalid_key_id',
    key_not_found: 'error_code_key_not_found',
    key_not_hideable: 'error_code_key_not_hideable',
    renewal_in_progress: 'error_code_renewal_in_progress',
    auto_renew_unavailable: 'error_code_auto_renew_unavailable',
    invalid_amount: 'error_code_invalid_amount',
    invalid_payment_method: 'error_code_invalid_payment_method',
    forbidden: 'error_code_forbidden',
};

/** Parses an API error body, tolerating the legacy plain-text form. */
export function parseAPIErrorBody(raw: string | undefined | null): APIErrorBody {
    const text = (raw || '').trim();
    if (!text) return {};
    try {
        const parsed = JSON.parse(text) as unknown;
        if (parsed && typeof parsed === 'object') {
            const candidate = parsed as APIErrorBody;
            return {
                code: typeof candidate.code === 'string' ? candidate.code : undefined,
                message: typeof candidate.message === 'string' ? candidate.message : undefined,
                pending_purchase: candidate.pending_purchase,
            };
        }
    } catch {
        // Older servers answer plain text; keep it as the message.
    }
    return { message: text };
}

/** The machine-readable code of an error, when it has one. */
export function apiErrorCode(error: unknown): string | undefined {
    if (error instanceof APIError) {
        return parseAPIErrorBody(error.body).code;
    }
    return undefined;
}

type Translate = (key: string, params?: Record<string, string | number>) => string;

/**
 * Localizes a failed API call.
 *
 * Order of preference: known code -> localized copy; unknown code with a server
 * message -> that message (it is at least specific); nothing usable -> the
 * caller's generic localized fallback.
 */
export function localizedAPIError(t: Translate, error: unknown, fallbackKey: string): string {
    if (error instanceof APIError) {
        const body = parseAPIErrorBody(error.body);
        if (body.code) {
            const key = CODE_TRANSLATION_KEYS[body.code];
            if (key) {
                const text = t(key);
                if (text && text !== key) return text;
            }
        }
        if (body.message) return body.message;
        // An API failure with neither a code nor a body says nothing a customer can
        // act on; APIError's own message is the synthesized "HTTP 500", which is
        // exactly the untranslated string U5 set out to remove.
        return t(fallbackKey);
    }
    if (error instanceof Error && error.message) {
        return error.message;
    }
    return t(fallbackKey);
}
