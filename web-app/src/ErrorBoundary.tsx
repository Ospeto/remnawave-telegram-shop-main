import React from 'react';

interface Props {
    children: React.ReactNode;
}

interface State {
    hasError: boolean;
    error: string;
}

export class ErrorBoundary extends React.Component<Props, State> {
    constructor(props: Props) {
        super(props);
        this.state = { hasError: false, error: '' };
    }

    static getDerivedStateFromError(error: Error): State {
        return { hasError: true, error: error.message };
    }

    componentDidCatch(error: Error, info: React.ErrorInfo) {
        console.error('React Error Boundary caught:', error, info);
    }

    public render() {
        if (this.state.hasError) {
            return (
                { this.state.error }
                    </pre >
                <button
                    onClick={() => window.location.reload()}
                    style={{
                        marginTop: '20px',
                        padding: '10px 20px',
                        backgroundColor: '#3b82f6',
                        color: 'white',
                        border: 'none',
                        borderRadius: '8px',
                        cursor: 'pointer'
                    }}
                >
                    Reload
                </button>
                </div >
            );
        }

        return this.props.children;
    }
}
