export class APIError extends Error {
    status: number;
    body: string;

    constructor(status: number, body: string) {
        super(body || `HTTP ${status}`);
        this.name = 'APIError';
        this.status = status;
        this.body = body;
    }
}

export async function fetchJSON<T>(input: RequestInfo | URL, init?: RequestInit): Promise<T> {
    const res = await fetch(input, init);
    if (!res.ok) {
        let body = '';
        try {
            body = await res.text();
        } catch {
            body = '';
        }
        throw new APIError(res.status, body);
    }
    return res.json() as Promise<T>;
}

export function isAPIStatus(error: unknown, status: number): boolean {
    return error instanceof APIError && error.status === status;
}
