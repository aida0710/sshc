import type { ReactNode } from "react";
import { formColumns } from "./FieldRow";

export type Fact = { label: string; value: ReactNode };

// FactRows lists facts that are only read, such as the version, in the rows of a form: the name on the
// left, the value on the right, and a line between rows. On a narrow screen the names take the width of the
// longest one; on a wide screen the column is as wide as FieldRow's, so a page that mixes the two lines them up.
export function FactRows({ facts }: { facts: readonly Fact[] }) {
  return (
    <dl className={`grid grid-cols-[minmax(0,max-content)_minmax(0,1fr)] gap-x-4 text-[13.5px] ${formColumns}`}>
      {facts.map(({ label, value }) => (
        <div key={label} className="col-span-2 grid grid-cols-subgrid border-t border-line py-2.5 first:border-t-0 first:pt-0.5 last:pb-0.5">
          <dt className="max-w-36 text-ink-muted">{label}</dt>
          <dd className="min-w-0 break-all text-ink">{value}</dd>
        </div>
      ))}
    </dl>
  );
}
