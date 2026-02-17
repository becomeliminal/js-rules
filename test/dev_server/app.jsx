import React, { useState } from "react";
import { createRoot } from "react-dom/client";

function Home() {
    return (
        <div>
            <h1>Home</h1>
            <p>Welcome to the dev server test.</p>
        </div>
    );
}

function About() {
    return (
        <div>
            <h1>About</h1>
            <p>This is a simple React SPA to test client-side routing.</p>
        </div>
    );
}

const routes = { "/": Home, "/about": About };

function App() {
    const [path, setPath] = useState(window.location.pathname);

    function navigate(to) {
        window.history.pushState(null, "", to);
        setPath(to);
    }

    // Handle browser back/forward
    React.useEffect(() => {
        const onPop = () => setPath(window.location.pathname);
        window.addEventListener("popstate", onPop);
        return () => window.removeEventListener("popstate", onPop);
    }, []);

    const Page = routes[path] || Home;

    return (
        <div>
            <nav style={{ display: "flex", gap: "1rem", padding: "1rem", borderBottom: "1px solid #ccc" }}>
                <button onClick={() => navigate("/")} style={{ fontWeight: path === "/" ? "bold" : "normal" }}>
                    Home
                </button>
                <button onClick={() => navigate("/about")} style={{ fontWeight: path === "/about" ? "bold" : "normal" }}>
                    About
                </button>
            </nav>
            <div style={{ padding: "1rem" }}>
                <Page />
            </div>
        </div>
    );
}

createRoot(document.getElementById("root")).render(<App />);
