import React from "react";
import { renderToString } from "react-dom/server";
import { App } from "test/react/components";

const html = renderToString(<App />);
console.log(html);
console.log("react test passed");
