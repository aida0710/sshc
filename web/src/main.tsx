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
    {/*
      **境界は provider の外側である。** theme も i18n も、その中で起きた例外の
      巻き添えで落ち得る。中に置けば、壊れた context をこの画面自身が引きに行く。
    */}
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
