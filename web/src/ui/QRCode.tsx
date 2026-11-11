// QRコードの周りに空ける余白（マスの数）。規格が、読み取りのために 4 マス以上を求める。
const quietZoneModules = 4;

// テーマの色を使わず、どのテーマでも白地に黒で描く。カメラが読むのは明暗の差で、
// ダークのテーマの色（暗い地に明るいマス）は読めない読み取り機がある。
const lightModuleColour = "#ffffff"; // palette-exempt
const darkModuleColour = "#000000"; // palette-exempt

// QRCode draws the modules of a QR code. rows are its lines from the top, each a run of "1" (dark) and
// "0" (light), without the margin.
export function QRCode({ rows, label, className = "" }: { rows: readonly string[]; label: string; className?: string }) {
  const side = rows.length + quietZoneModules * 2;
  const darkModules = rows
    .flatMap((row, y) =>
      [...row].map((module, x) => (module === "1" ? `M${x + quietZoneModules} ${y + quietZoneModules}h1v1h-1z` : "")),
    )
    .join("");
  return (
    <svg role="img" aria-label={label} viewBox={`0 0 ${side} ${side}`} shapeRendering="crispEdges" className={className}>
      <rect width={side} height={side} fill={lightModuleColour} />
      <path d={darkModules} fill={darkModuleColour} />
    </svg>
  );
}
