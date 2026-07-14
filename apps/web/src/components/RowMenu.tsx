import { ContextMenu } from "@base-ui-components/react/context-menu";
import type { ReactElement } from "react";

export interface MenuAction {
  label: string;
  onSelect: () => void;
  danger?: boolean;
}

// RowMenu attaches a Base UI right-click context menu to an element. The trigger
// renders as the child itself (no wrapper), so styling/layout is unaffected.
export function RowMenu({ actions, children }: { actions: MenuAction[]; children: ReactElement }) {
  return (
    <ContextMenu.Root>
      <ContextMenu.Trigger render={children as ReactElement<Record<string, unknown>>} />
      <ContextMenu.Portal>
        <ContextMenu.Positioner className="ctx-positioner" sideOffset={2}>
          <ContextMenu.Popup className="ctx-popup">
            {actions.map((a, i) => (
              <ContextMenu.Item
                key={i}
                className={`ctx-item${a.danger ? " ctx-item--danger" : ""}`}
                onClick={a.onSelect}
              >
                {a.label}
              </ContextMenu.Item>
            ))}
          </ContextMenu.Popup>
        </ContextMenu.Positioner>
      </ContextMenu.Portal>
    </ContextMenu.Root>
  );
}

export function openOnGitHub(url?: string) {
  if (url) window.open(url, "_blank", "noopener,noreferrer");
}
