import { lazy, Suspense } from 'react';
import { BrowserRouter, Routes, Route } from 'react-router-dom';
import { Home } from './pages/Home';
import { Plans } from './pages/Plans';
import { Checkout } from './pages/Checkout';
import { Wallet } from './pages/Wallet';

import { ThemeProvider } from './lib/ThemeProvider';
import { UserDataProvider } from './lib/userData';
import { RouteLoadingSkeleton } from './components/RouteLoadingSkeleton';

// The admin and reseller surface is a large, rarely visited part of the app:
// it is split out of the first-paint bundle and fetched on demand.
const AdminLayout = lazy(() => import('./pages/AdminLayout').then((m) => ({ default: m.AdminLayout })));
const AdminHub = lazy(() => import('./pages/AdminHub').then((m) => ({ default: m.AdminHub })));
const AdminFinance = lazy(() => import('./pages/AdminFinance').then((m) => ({ default: m.AdminFinance })));
const AdminPlans = lazy(() => import('./pages/AdminPlans').then((m) => ({ default: m.AdminPlans })));
const AdminPromos = lazy(() => import('./pages/AdminPromos').then((m) => ({ default: m.AdminPromos })));
const AdminResellers = lazy(() => import('./pages/AdminResellers').then((m) => ({ default: m.AdminResellers })));
const ResellerAccount = lazy(() => import('./pages/ResellerAccount').then((m) => ({ default: m.ResellerAccount })));

function App() {
    return (
        <ThemeProvider>
            <UserDataProvider>
                <BrowserRouter>
                    <Suspense fallback={<RouteLoadingSkeleton />}>
                        <Routes>
                            <Route path="/" element={<Home />} />
                            <Route path="/admin" element={<AdminLayout />}>
                                <Route index element={<AdminHub />} />
                                <Route path="finance" element={<AdminFinance />} />
                                <Route path="plans" element={<AdminPlans />} />
                                <Route path="promos" element={<AdminPromos />} />
                            </Route>
                            <Route path="/admin/resellers" element={<AdminResellers />} />
                            <Route path="/reseller/account" element={<ResellerAccount />} />
                            <Route path="/plans" element={<Plans />} />
                            <Route path="/wallet" element={<Wallet />} />
                            <Route path="/checkout" element={<Checkout />} />
                            <Route path="/checkout/:planIndex" element={<Checkout />} />
                        </Routes>
                    </Suspense>
                </BrowserRouter>
            </UserDataProvider>
        </ThemeProvider>
    );
}

export default App;
