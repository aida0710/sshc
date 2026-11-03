// SwitchMark draws a setting that is on or off as a switch. It is only the picture: the control around it (a
// menu item, a checkbox) carries the state for assistive technology.
export function SwitchMark({ on }: { on: boolean }) {
  return (
    <span aria-hidden="true" className={`relative h-[18px] w-[30px] shrink-0 rounded-full transition-colors ${on ? "bg-accent" : "bg-ink/20"}`}>
      <span className={`absolute top-0.5 size-3.5 rounded-full bg-card transition-[left] ${on ? "left-[14px]" : "left-0.5"}`} />
    </span>
  );
}
