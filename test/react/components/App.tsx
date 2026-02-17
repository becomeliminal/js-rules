import React, { useState } from "react";
import { Counter } from "./Counter";
import { Greeting } from "./Greeting";

export function App() {
  const [name, setName] = useState("World");

  return (
    <div>
      <Greeting name={name} />
      <Counter />
      <input
        value={name}
        onChange={(e) => setName(e.target.value)}
        placeholder="Enter your name"
      />
    </div>
  );
}
