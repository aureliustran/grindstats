import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

// tokens.css must load before globals.css — it defines the values globals.css
// (and Tailwind, via tailwind.config.js) consume. See docs/design-system.md §2.
import "./styles/tokens.css";
import "./styles/globals.css";
import "./i18n";

import App from "./App";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <App />
  </StrictMode>,
);
