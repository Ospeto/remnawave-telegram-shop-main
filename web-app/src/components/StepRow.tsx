import { ReactNode } from 'react';

interface StepRowProps {
    number: number;
    children: ReactNode;
}

export function StepRow({ number, children }: StepRowProps) {
    return (
        <div className="step-row">
            <span
                className="step-number"
                aria-label={`Step ${number}`}
                style={{ width: 24, height: 24, fontSize: 12 }}
            >
                {number}
            </span>
            <span className="step-text">{children}</span>
        </div>
    );
}
