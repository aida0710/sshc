// 知らせの枠の色。地はカードの色に意味の色を少しだけ混ぜ、枠を同じ色で引く。字は墨と淡い字のまま読ませ、
// 意味の色は字に使わない（赤い字の段落は読みにくい）。ターミナルの知らせの枠（NoticeFrame）と、
// 画面の中に置く知らせ（Notice）が同じ色を使う。
export const noticeFrameColours = {
  // 利用者の答えを待つ問い（ホスト鍵など）。
  accent: "border-accent/50 bg-[color-mix(in_srgb,var(--ui-accent)_10%,var(--ui-card))]",
  // 注意。
  notice: "border-notice-line bg-[color-mix(in_srgb,var(--ui-notice-ink)_8%,var(--ui-card))]",
  // 失敗と、止まったもの。
  danger: "border-danger/45 bg-[color-mix(in_srgb,var(--ui-danger)_9%,var(--ui-card))]",
  // 済んだこと。
  success: "border-live/40 bg-[color-mix(in_srgb,var(--ui-live)_8%,var(--ui-card))]",
} as const;
