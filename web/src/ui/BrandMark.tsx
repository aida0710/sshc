import { useId } from "react";

export function BrandMark({ className = "h-7 w-7" }: { className?: string }) {
  const gradientId = `sshc-brand-gradient-${useId().replaceAll(":", "")}`;

  return (
    <svg
      aria-hidden="true"
      data-sshc-brand-mark="true"
      className={`shrink-0 ${className}`}
      viewBox="0 0 512 512"
    >
      <defs>
        <linearGradient id={gradientId} x1="0" y1="0" x2="1" y2="1">
          <stop offset="0%" stopColor="var(--ui-brand-gradient-start)" />
          <stop offset="58%" stopColor="var(--ui-brand-gradient-middle)" />
          <stop offset="100%" stopColor="var(--ui-brand-gradient-end)" />
        </linearGradient>
      </defs>
      <rect width="512" height="512" rx="112" fill="var(--ui-brand-background)" />
      <path
        d="M136 166H244L288 210H376V346H268L224 302H136Z"
        fill="none"
        stroke={`url(#${gradientId})`}
        strokeWidth="28"
        strokeLinejoin="round"
        strokeLinecap="round"
      />
      <rect x="304" y="248" width="24" height="52" rx="3" fill="var(--ui-brand-cursor)" />
    </svg>
  );
}
