import { MemoryRouter, Routes, Route } from 'react-router-dom';
import { Home } from './pages/Home';
import { Plans } from './pages/Plans';
import { Checkout } from './pages/Checkout';

function App() {
    return (
        <MemoryRouter>
            <Routes>
                <Route path="/" element={<Home />} />
                <Route path="/plans" element={<Plans />} />
                <Route path="/checkout/:planIndex" element={<Checkout />} />
            </Routes>
        </MemoryRouter>
    );
}

export default App;
