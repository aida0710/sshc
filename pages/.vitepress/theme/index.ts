import docsTheme from "@aida0710/app-docs-theme/theme";
import type { Theme } from "vitepress";
import MermaidDiagram from "./MermaidDiagram.vue";
import "./colors.css";
import "./mermaid.css";

const theme: Theme = {
  extends: docsTheme,
  enhanceApp({ app }) {
    app.component("MermaidDiagram", MermaidDiagram);
  },
};

export default theme;
