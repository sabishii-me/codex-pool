import * as AlertDialogPrimitive from "@radix-ui/react-alert-dialog";
import type { ReactNode } from "react";

export function ConfirmDialog({ open, onOpenChange, title, description, confirmLabel, tone = "danger", busy = false, onConfirm, children }: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description: string;
  confirmLabel: string;
  tone?: "danger" | "default";
  busy?: boolean;
  onConfirm: () => void;
  children?: ReactNode;
}) {
  return <AlertDialogPrimitive.Root open={open} onOpenChange={onOpenChange}>
    {children ? <AlertDialogPrimitive.Trigger asChild>{children}</AlertDialogPrimitive.Trigger> : null}
    <AlertDialogPrimitive.Portal>
      <AlertDialogPrimitive.Overlay className="ui-dialog-overlay" />
      <AlertDialogPrimitive.Content className="ui-alert-dialog">
        <div className={`ui-dialog-mark ${tone}`}>{tone === "danger" ? "!" : "↻"}</div>
        <AlertDialogPrimitive.Title className="ui-dialog-title">{title}</AlertDialogPrimitive.Title>
        <AlertDialogPrimitive.Description className="ui-dialog-description">{description}</AlertDialogPrimitive.Description>
        <div className="ui-dialog-actions">
          <AlertDialogPrimitive.Cancel className="secondary-button" disabled={busy}>Cancel</AlertDialogPrimitive.Cancel>
          <AlertDialogPrimitive.Action className={tone === "danger" ? "destructive-button" : "primary-button"} disabled={busy} onClick={event => { event.preventDefault(); onConfirm(); }}>{busy ? "Working…" : confirmLabel}</AlertDialogPrimitive.Action>
        </div>
      </AlertDialogPrimitive.Content>
    </AlertDialogPrimitive.Portal>
  </AlertDialogPrimitive.Root>;
}
