import DefaultTheme from "vitepress/theme";
import type { Theme } from "vitepress";
import "@fontsource-variable/noto-sans-jp";
import MermaidDiagram from "./MermaidDiagram.vue";
import "./style.css";

const theme: Theme = {
  extends: DefaultTheme,
  enhanceApp({ app }) {
    app.component("MermaidDiagram", MermaidDiagram);
  },
};

export default theme;
