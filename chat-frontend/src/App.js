import React from 'react';
import ChatRoom from './components/ChatRoom';

function App() {
    return (
        <div className="min-h-screen bg-gray-100">
            <div className="container mx-auto h-screen p-4 flex flex-col">
                <header className="text-center py-2">
                    <h1 className="text-2xl font-bold text-gray-800">Real-time Chat Room</h1>
                </header>

                {/* Main chat component with flex-grow to fill available space */}
                <main className="flex-grow">
                    <ChatRoom />
                </main>

                <footer className="py-2 text-center text-sm text-gray-500">
                    <p>© {new Date().getFullYear()} Chat Room App</p>
                </footer>
            </div>
        </div>
    );
}

export default App;