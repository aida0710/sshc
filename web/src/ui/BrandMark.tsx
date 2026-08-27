export function BrandMark({ className = "h-7 w-7" }: { className?: string }) {
  return (
    <svg
      aria-hidden="true"
      data-sshc-brand-mark="true"
      className={`shrink-0 ${className}`}
      viewBox="0 0 512 512"
    >
      <rect width="512" height="512" rx="112" fill="var(--ui-brand-background)" />
      <path
        d="M136 166H244L288 210H376V346H268L224 302H136Z"
        fill="none"
        stroke="var(--ui-brand-mark)"
        strokeWidth="28"
        strokeLinejoin="round"
        strokeLinecap="round"
      />
      <rect x="304" y="248" width="24" height="52" rx="3" fill="var(--ui-brand-cursor)" />
    </svg>
  );
}
