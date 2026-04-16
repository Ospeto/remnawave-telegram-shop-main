interface LoadingScreenProps {
    message?: string;
}

export function LoadingScreen({ message }: LoadingScreenProps) {
    return (
        <div className="screen-center" role="status" aria-live="polite" aria-busy="true">
            <div className="spinner" />
            {message && <span className="text-hint" style={{ fontSize: 13 }}>{message}</span>}
        </div>
    );
}
