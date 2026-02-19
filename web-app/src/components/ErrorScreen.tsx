interface ErrorScreenProps {
    message: string;
    onRetry?: () => void;
    retryLabel?: string;
}

export function ErrorScreen({ message, onRetry, retryLabel = 'Retry' }: ErrorScreenProps) {
    return (
        <div className="screen-center" style={{ padding: 24, textAlign: 'center' }}>
            <div style={{ fontSize: 48 }}>⚠️</div>
            <p style={{ color: '#ff3b30', margin: 0, fontSize: 14 }}>{message}</p>
            {onRetry && (
                <button className="btn-secondary" onClick={onRetry} style={{ marginTop: 8 }}>
                    {retryLabel}
                </button>
            )}
        </div>
    );
}
