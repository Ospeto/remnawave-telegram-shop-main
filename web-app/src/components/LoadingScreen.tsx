interface LoadingScreenProps {
    message?: string;
}

export function LoadingScreen({ message }: LoadingScreenProps) {
    return (
        <div className="screen-center">
            <div className="spinner" />
            {message && <span className="text-hint" style={{ fontSize: 13 }}>{message}</span>}
        </div>
    );
}
