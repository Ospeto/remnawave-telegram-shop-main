interface SessionExpiredScreenProps {
    title: string;
    message: string;
    reloadLabel: string;
    closeLabel: string;
    onReload: () => void;
    onClose: () => void;
}

export function SessionExpiredScreen({
    title,
    message,
    reloadLabel,
    closeLabel,
    onReload,
    onClose,
}: SessionExpiredScreenProps) {
    return (
        <div className="screen-center" style={{ padding: 24, textAlign: 'center' }}>
            <div style={{ fontSize: 48 }} aria-hidden="true">Session</div>
            <h1 style={{ fontSize: 20, fontWeight: 700, margin: 0 }}>{title}</h1>
            <p className="text-hint" style={{ margin: 0, fontSize: 14 }}>{message}</p>
            <div style={{ display: 'flex', flexDirection: 'column', gap: 8, width: '100%', maxWidth: 340 }}>
                <button className="btn-primary" onClick={onReload}>{reloadLabel}</button>
                <button className="btn-secondary" onClick={onClose}>{closeLabel}</button>
            </div>
        </div>
    );
}
