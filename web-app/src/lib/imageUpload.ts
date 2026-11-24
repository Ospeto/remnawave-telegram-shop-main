/**
 * Receipt-image preparation for upload.
 *
 * A12: phone cameras produce 3-10 MB receipts. The server now gives the upload
 * route a long read deadline, but a customer on a slow mobile link still pays
 * every second of that transfer, and Telegram's WebView can drop the request
 * before it completes. Shrinking the picture here means the bytes actually sent
 * are ~1.5 MB, while the receipt text stays readable for OCR.
 */

/** Target ceiling for the encoded upload. */
export const MAX_UPLOAD_BYTES = 1_500_000;

/** Longest edge after downscaling; receipts stay legible well below this. */
export const MAX_UPLOAD_EDGE = 1600;

/** JPEG quality steps tried in order until the file fits the ceiling. */
const QUALITY_STEPS = [0.9, 0.8, 0.7, 0.6, 0.5];

export interface PreparedUpload {
    /** The file to send: the downscaled one, or the original when it already fits. */
    file: File;
    /** True when the bytes were re-encoded. */
    downscaled: boolean;
}

function uploadFileName(): string {
    // The receipt is re-encoded as JPEG, so the name has to match the bytes the
    // server will sniff; the original camera name is not worth keeping.
    return 'receipt.jpg';
}

/**
 * Downscales a receipt photo so the upload fits MAX_UPLOAD_BYTES.
 *
 * Anything that cannot be processed (no canvas support, an undecodable file, a
 * re-encode that somehow came out larger) returns the original file unchanged:
 * a preparation failure must never block the customer from uploading.
 */
export async function prepareReceiptUpload(file: File): Promise<PreparedUpload> {
    if (file.size <= MAX_UPLOAD_BYTES) {
        return { file, downscaled: false };
    }

    if (typeof document === 'undefined' || typeof createImageBitmap !== 'function') {
        return { file, downscaled: false };
    }

    let bitmap: ImageBitmap | null = null;
    try {
        bitmap = await createImageBitmap(file);
        const scale = Math.min(1, MAX_UPLOAD_EDGE / Math.max(bitmap.width, bitmap.height));
        const width = Math.max(1, Math.round(bitmap.width * scale));
        const height = Math.max(1, Math.round(bitmap.height * scale));

        const canvas = document.createElement('canvas');
        canvas.width = width;
        canvas.height = height;
        const context = canvas.getContext('2d');
        if (!context) {
            return { file, downscaled: false };
        }
        // White background: transparent PNG receipts otherwise turn black as JPEG.
        context.fillStyle = '#ffffff';
        context.fillRect(0, 0, width, height);
        context.drawImage(bitmap, 0, 0, width, height);

        for (const quality of QUALITY_STEPS) {
            const blob = await canvasToBlob(canvas, quality);
            if (!blob) continue;
            if (blob.size <= MAX_UPLOAD_BYTES || quality === QUALITY_STEPS[QUALITY_STEPS.length - 1]) {
                if (blob.size >= file.size) {
                    return { file, downscaled: false };
                }
                return { file: new File([blob], uploadFileName(), { type: 'image/jpeg' }), downscaled: true };
            }
        }
        return { file, downscaled: false };
    } catch {
        return { file, downscaled: false };
    } finally {
        bitmap?.close?.();
    }
}

function canvasToBlob(canvas: HTMLCanvasElement, quality: number): Promise<Blob | null> {
    return new Promise((resolve) => {
        if (typeof canvas.toBlob !== 'function') {
            resolve(null);
            return;
        }
        canvas.toBlob((blob) => resolve(blob), 'image/jpeg', quality);
    });
}
