import { ReactNode } from 'react';

interface TipBoxProps {
    variant: 'info' | 'warning' | 'success';
    icon: string;
    children: ReactNode;
    style?: React.CSSProperties;
}

export function TipBox({ variant, icon, children, style }: TipBoxProps) {
    return (
        <div className={`tip-box tip-box-${variant}`} style={style}>
            <span className="tip-icon" aria-hidden="true">{icon}</span>
            <span>{children}</span>
        </div>
    );
}
