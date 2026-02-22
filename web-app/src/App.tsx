import { MemoryRouter, Routes, Route } from 'react-router-dom';
import { Home } from './pages/Home';
import { Plans } from './pages/Plans';
import { Checkout } from './pages/Checkout';
import { Wallet } from './pages/Wallet';

import { ThemeProvider } from './lib/ThemeProvider';

function App() {
    return (
        <ThemeProvider>
            <MemoryRouter>
                <Routes>
                    <Route path="/" element={<Home />} />
                    <Route path="/plans" element={<Plans />} />
                    <Route path="/wallet" element={<Wallet />} />
                    <Route path="/checkout/:planIndex" element={<Checkout />} />
                </Routes>
            </MemoryRouter>
        </ThemeProvider>
    );
}

export default App;
