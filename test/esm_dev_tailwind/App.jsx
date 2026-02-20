import { useState } from "react";

export function App() {
    const [count, setCount] = useState(0);

    return (
        <div className="min-h-screen bg-gradient-to-br from-blue-500 to-purple-600 p-8">
            <div className="max-w-md mx-auto bg-white rounded-xl shadow-lg p-6">
                <h1 className="text-2xl font-bold text-gray-800 mb-4">
                    ESM Dev + Tailwind
                </h1>
                <p className="text-gray-600 mb-4">
                    If you see styled content, Tailwind is working in ESM dev mode.
                </p>
                <div className="flex items-center gap-4">
                    <button
                        className="px-4 py-2 bg-blue-500 text-white rounded hover:bg-blue-600 transition-colors"
                        onClick={() => setCount(c => c + 1)}
                    >
                        Count: {count}
                    </button>
                    <span className="text-sm text-gray-400">
                        Click to test React interactivity
                    </span>
                </div>
            </div>
        </div>
    );
}
