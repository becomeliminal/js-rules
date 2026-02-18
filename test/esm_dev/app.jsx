import React, { useState } from "react";
import { createRoot } from "react-dom/client";
import "./style.css";

function App() {
    const [count, setCount] = useState(0);
    return (<div>
        <h1>ESM Dev Server</h1>
        <p>Count: {count}</p>
        <button onClick={() => setCount(c => c + 1)}>+</button>
    </div>);
}

createRoot(document.getElementById("root")).render(<App />);
