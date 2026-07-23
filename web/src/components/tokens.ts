export const semanticTokens = {
  color: {
    canvas: "var(--color-canvas)",
    surface: "var(--color-surface)",
    elevated: "var(--color-surface-elevated)",
    content: "var(--color-content-primary)",
    muted: "var(--color-content-muted)",
    accent: "var(--color-accent-primary)",
    success: "var(--color-accent-success)",
    warning: "var(--color-accent-warning)",
    info: "var(--color-accent-info)",
  },
  radius: { control: "var(--radius-control)", card: "var(--radius-card)" },
  space: { 1: "var(--space-1)", 2: "var(--space-2)", 3: "var(--space-3)", 4: "var(--space-4)", 5: "var(--space-5)", 6: "var(--space-6)", 8: "var(--space-8)" },
} as const;
