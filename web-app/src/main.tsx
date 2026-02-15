import React from 'react'
import ReactDOM from 'react-dom/client'
import { ErrorBoundary } from './ErrorBoundary'
import App from './App.tsx'
import './index.css'

console.log('[MiniApp] Starting render...');
console.log('[MiniApp] Telegram available:', !!window.Telegram?.WebApp);
console.log('[MiniApp] initData:', window.Telegram?.WebApp?.initData ? 'present' : 'empty');

ReactDOM.createRoot(document.getElementById('root')!).render(
    <React.StrictMode>
        <ErrorBoundary>
            <App />
        </ErrorBoundary>
    </React.StrictMode>,
)

console.log('[MiniApp] Render call complete');
