import SiteAdmin from "./SiteAdmin";
import { usesSiteAdmin } from "./api";
import { sitePath } from "./site-path";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import "./index.css";
import App from "./App.tsx";
import SiteApp from "./SiteApp.tsx";

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    {/^\/admin\/?$/.test(sitePath()) ? (
      usesSiteAdmin ? (
        <SiteAdmin />
      ) : (
        <App />
      )
    ) : (
      <SiteApp />
    )}
  </StrictMode>,
);
