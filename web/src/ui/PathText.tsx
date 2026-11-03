// PathText draws a path that may not fit. A long path loses its start, not the name it ends in: the right-to-left box
// puts the ellipsis on the left, and the bdi keeps the path itself in reading order.
export function PathText({ path, className = "" }: { path: string; className?: string }) {
  return (
    <span dir="rtl" className={`block min-w-0 truncate text-left ${className}`}>
      <bdi dir="ltr">{path}</bdi>
    </span>
  );
}
