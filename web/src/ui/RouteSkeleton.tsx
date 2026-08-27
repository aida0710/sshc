type RouteSkeletonProps = {
  kind?: "section" | "terminal";
};

function SkeletonLine({ className }: { className: string }) {
  return <div className={`sshc-skeleton rounded-md ${className}`} />;
}

export function RouteSkeleton({ kind = "section" }: RouteSkeletonProps) {
  if (kind === "terminal") {
    return (
      <div aria-hidden="true" className="h-full bg-term-bg p-5">
        <div className="flex max-w-3xl flex-col gap-3 opacity-55">
          <SkeletonLine className="h-3 w-48" />
          <SkeletonLine className="h-3 w-72" />
          <SkeletonLine className="h-3 w-56" />
        </div>
      </div>
    );
  }

  return (
    <div aria-hidden="true" className="h-full overflow-hidden p-6">
      <div className="mx-auto flex max-w-6xl flex-col gap-6">
        <div className="flex flex-col gap-2">
          <SkeletonLine className="h-7 w-52" />
          <SkeletonLine className="h-3.5 w-full max-w-xl" />
        </div>
        <div className="sshc-card grid gap-px overflow-hidden rounded-md bg-line sm:grid-cols-3">
          {Array.from({ length: 3 }, (_, index) => (
            <div key={index} className="flex flex-col gap-3 bg-card p-4">
              <SkeletonLine className="h-3 w-24" />
              <SkeletonLine className="h-6 w-16" />
            </div>
          ))}
        </div>
        <div className="sshc-card flex flex-col gap-4 rounded-md bg-card p-4">
          <SkeletonLine className="h-4 w-36" />
          <SkeletonLine className="h-10 w-full" />
          <SkeletonLine className="h-14 w-full" />
          <SkeletonLine className="h-14 w-full" />
        </div>
      </div>
    </div>
  );
}
