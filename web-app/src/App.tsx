import { BrowserRouter, Routes, Route } from 'react-router-dom';
import { Home } from './pages/Home';
import { Plans } from './pages/Plans';
import { Checkout } from './pages/Checkout';
import { Wallet } from './pages/Wallet';
import { AdminPromos } from './pages/AdminPromos';
import { AdminPlans } from './pages/AdminPlans';
import { AdminFinance } from './pages/AdminFinance';

import { ThemeProvider } from './lib/ThemeProvider';

function App() {
    return (
        <ThemeProvider>
            <BrowserRouter>
                <Routes>
                    <Route path="/" element={<Home />} />
                    <Route path="/admin/promos" element={<AdminPromos />} />
                    <Route path="/admin/plans" element={<AdminPlans />} />
                    <Route path="/admin/finance" element={<AdminFinance />} />
                    <Route path="/plans" element={<Plans />} />
                    <Route path="/wallet" element={<Wallet />} />
                    <Route path="/checkout" element={<Checkout />} />
                    <Route path="/checkout/:planIndex" element={<Checkout />} />
                </Routes>
            </BrowserRouter>
        </ThemeProvider>
    );
}

export default App;
