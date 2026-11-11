import { menuItemClass } from "../ui/menuStyle";

export type SFTPMenuAction = {
  key: string;
  label: string;
  danger?: boolean;
  disabled?: boolean;
  run: () => void;
};

export const phoneMenuItemClass = "block min-h-12 w-full rounded-md px-3 py-3 text-left text-base hover:bg-hover focus:bg-select-fill";

export function SFTPMenuActionList({ actions, mobile = false }: { actions: SFTPMenuAction[]; mobile?: boolean }) {
  return (
    <>
      {actions.map((action) => (
        <button
          key={action.key}
          type="button"
          role="menuitem"
          disabled={action.disabled === true}
          onClick={action.run}
          className={mobile ? `${phoneMenuItemClass} ${action.danger === true ? "text-danger" : ""}` : menuItemClass(action.danger === true ? "danger" : undefined)}
        >
          {action.label}
        </button>
      ))}
    </>
  );
}
