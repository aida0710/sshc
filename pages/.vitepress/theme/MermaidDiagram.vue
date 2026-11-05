<script setup lang="ts">
import { onMounted, ref, useId, watch } from "vue";
import { useData } from "vitepress";

const props = defineProps<{ source: string }>();

const { isDark } = useData();
const svg = ref("");
const failed = ref(false);
const diagramId = `mermaid-${useId()}`;

// Mermaidは色をCSS変数のまま受け取れないので、サイトの配色を実際の値にして渡す。
function siteColor(token: string): string {
  return getComputedStyle(document.documentElement).getPropertyValue(token).trim();
}

function diagramConfig() {
  const text = siteColor("--vp-c-text-1");
  const line = siteColor("--vp-c-text-2");
  const surface = siteColor("--vp-c-bg-elv");
  const border = siteColor("--vp-c-border");
  const band = siteColor("--vp-c-bg-soft");
  return {
    startOnLoad: false,
    theme: "base" as const,
    look: "classic" as const,
    darkMode: isDark.value,
    fontFamily: getComputedStyle(document.body).fontFamily,
    // スマホの幅でも字が読める大きさに収まるよう、箱の間隔を詰める。
    flowchart: { useMaxWidth: true, wrappingWidth: 260, nodeSpacing: 20, rankSpacing: 36, padding: 10, diagramPadding: 4 },
    state: { useMaxWidth: true, nodeSpacing: 16, rankSpacing: 36, padding: 6 },
    // 縦に長くなるので、下に同じ参加者の列を繰り返さない。スマホの幅に収まるよう箱を小さくする。
    sequence: {
      useMaxWidth: true,
      mirrorActors: false,
      width: 96,
      height: 42,
      actorMargin: 16,
      messageMargin: 30,
      boxMargin: 6,
      diagramMarginX: 2,
      diagramMarginY: 4,
    },
    themeVariables: {
      fontSize: "14px",
      dropShadow: "none",
      background: siteColor("--vp-c-bg"),
      primaryColor: surface,
      primaryTextColor: text,
      primaryBorderColor: border,
      secondaryColor: band,
      tertiaryColor: band,
      lineColor: line,
      textColor: text,
      actorBkg: surface,
      actorBorder: border,
      actorTextColor: text,
      actorLineColor: border,
      signalColor: line,
      signalTextColor: text,
      labelBoxBkgColor: band,
      labelBoxBorderColor: border,
      labelTextColor: text,
      loopTextColor: text,
      noteBkgColor: band,
      noteBorderColor: border,
      noteTextColor: text,
      activationBkgColor: band,
      activationBorderColor: border,
      clusterBkg: band,
      clusterBorder: border,
      edgeLabelBackground: siteColor("--vp-c-bg"),
    },
  };
}

// URLの#で開いたページでは、上にある図が描かれると見出しが下へずれるので、もう一度合わせる。
function scrollBackToLinkedHeading(): void {
  const heading = document.getElementById(decodeURIComponent(location.hash.slice(1)));
  heading?.scrollIntoView();
}

async function drawDiagram(): Promise<void> {
  // 図のあるページだけが、大きなMermaidの本体を読み込む。
  const { default: mermaid } = await import("mermaid");
  await document.fonts.ready;
  mermaid.initialize(diagramConfig());
  try {
    const rendered = await mermaid.render(diagramId, decodeURIComponent(props.source));
    svg.value = rendered.svg;
  } catch (error) {
    failed.value = true;
    console.error(error);
  }
}

onMounted(async () => {
  await drawDiagram();
  if (location.hash) scrollBackToLinkedHeading();
});
watch(isDark, drawDiagram);
</script>

<template>
  <figure class="mermaid-diagram">
    <pre v-if="failed">{{ decodeURIComponent(source) }}</pre>
    <div v-else v-html="svg" />
  </figure>
</template>
