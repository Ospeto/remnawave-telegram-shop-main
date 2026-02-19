import { ReactNode } from 'react';

interface TipBoxProps {
    variant: 'info' | 'warning' | 'success';
    icon: string;
    children: ReactNode;
    style?: React.CSSProperties;
    allowHtml?: boolean;
}

export function TipBox({ variant, icon, children, style, allowHtml }: TipBoxProps) {
    return (
        <div className={`tip-box tip-box-${variant}`} style={style}>
            <span className="tip-icon" aria-hidden="true">{icon}</span>
            {allowHtml && typeof children === 'string' ? (
                <span dangerouslySetInnerHTML={{ __html: children }} />
            ) : (
                <span>{children}</span>
            )}
        </div>
    );
}
