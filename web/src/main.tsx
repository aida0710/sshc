import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { apiClient } from "./api/client";
import { App } from "./App";
import "./index.css";
import { CrashBoundary } from "./shell/CrashBoundary";
import { bootstrapSession } from "./session/bootstrap";
import { LanguageProvider } from "./i18n/context";
import { ThemeProvider } from "./theme/context";

const root = document.getElementById("root");
if (!root) throw new Error("root element missing");

const sessionPromise = bootstrapSession(window.location, window.history, window.fetch.bind(window));

createRoot(root).render(
  <StrictMode>

    <CrashBoundary>
    <ThemeProvider>
      <LanguageProvider>
        <App
          bootstrap={() => sessionPromise}
          health={() => apiClient.health()}
        />
      </LanguageProvider>
    </ThemeProvider>
    </CrashBoundary>
  </StrictMode>,
);
