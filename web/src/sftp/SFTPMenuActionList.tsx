import { MenuItem } from "../ui/MenuItems";

export type SFTPMenuAction = {
  key: string;
  label: string;
  danger?: boolean;
  disabled?: boolean;
  run: () => void;
};

export const phoneMenuItemClass = "block min-h-12 w-full rounded-md px-3 py-3 text-left text-base hover:bg-hover focus:bg-select-fill";

// SFTPMenuActionList は SFTP のメニューの項目を並べる。PC ではメニューの枠（MenuPanel）の項目、スマホでは下から
// 出るシート（SFTPActionSheet）の、指で押しやすい高い項目にする。
export function SFTPMenuActionList({ actions, mobile = false }: { actions: SFTPMenuAction[]; mobile?: boolean }) {
  if (!mobile) {
    return (
      <>
        {actions.map((action) => (
          <MenuItem key={action.key} tone={action.danger === true ? "danger" : undefined} disabled={action.disabled === true} onSelect={action.run}>
            {action.label}
          </MenuItem>
        ))}
      </>
    );
  }
  return (
    <>
      {actions.map((action) => (
        <button
          key={action.key}
          type="button"
          role="menuitem"
          disabled={action.disabled === true}
          onClick={action.run}
          className={`${phoneMenuItemClass} ${action.danger === true ? "text-danger" : ""}`}
        >
          {action.label}
        </button>
      ))}
    </>
  );
}
