import { useState } from "react";
export function Counter() {
    const [count, setCount] = useState(0);
    return (<div>
        <p>Count: {count}</p>
        <button onClick={() => setCount(c => c + 1)}>+</button>
    </div>);
}
