import { Outlet } from 'react-router-dom';

export function AdminLayout() {
    return (
        <div
            className="admin-layout"
            style={{
                minHeight: '100%',
                width: '100%',
            }}
        >
            <Outlet />
        </div>
    );
}
