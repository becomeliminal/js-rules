import { createRoot } from "react-dom/client";
import { Greeting } from "./components";

createRoot(document.getElementById("root")!).render(<Greeting name="World" />);
