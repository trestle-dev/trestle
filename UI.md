# Administration UI contract

The dashboard is a dark, dense-but-calm work surface built with Nift-authored
HTML and vanilla CSS/JavaScript. It is not a marketing page and must not inherit
document scrolling conventions.

## Fixed-viewport invariant

The page itself never scrolls in either direction. The shell always fits the
visual viewport; only deliberate child regions scroll.

```css
html,
body {
  width: 100%;
  height: 100%;
  margin: 0;
  overflow: hidden;
}

.app-shell {
  width: 100vw;
  height: 100dvh;
  display: grid;
  grid-template-rows: auto minmax(0, 1fr);
  overflow: hidden;
}

.workspace {
  min-width: 0;
  min-height: 0;
  overflow: hidden;
}
```

Every grid/flex ancestor between the shell and a scroll pane needs the relevant
`min-width: 0` and `min-height: 0`. Use `overflow: auto` only on named content
panes, tables, editors, drawers, dialogs, logs, or timelines. Avoid `100vw` in
nested content, unbounded min-content columns, and body-mounted dropdowns that
change document dimensions.

Automated browser checks must assert `scrollWidth <= clientWidth` and
`scrollHeight <= clientHeight` for the root at supported desktop, laptop, and
narrow viewport sizes, including open dialogs and long validation messages.

## Layout direction

- Top bar: project, environment/connection state, search/command entry, actor.
- Navigation rail: collections, users, files, realtime, integrations, jobs,
  audit, backups, and settings.
- Workspace: resizable list/detail or editor panes with local scroll ownership.
- Inspector/drawer: schema, generated request, rule explanation, event/job detail.

At narrow widths, secondary panes become focus-managed drawers rather than
forcing horizontal page overflow. Data grids use sticky headers, bounded column
widths, virtualization when measured need justifies it, and an internal scroller.

## Vanilla frontend architecture

Use small ES modules organized around routes, API transport, state, and reusable
DOM behavior. Prefer semantic HTML, templates, custom events, and plain functions.
Do not introduce a framework, framework-shaped dependency, virtual DOM, or a
bespoke component runtime. Dependencies must solve a bounded problem and survive
security/license review.

Server APIs remain the source of truth. Optimistic UI is allowed only when it can
reconcile conflicts and display failure. Routes should be linkable where doing so
does not expose secrets; unsaved edits need explicit navigation protection.

## Theme and accessibility

Author dark mode first with tokens for surface, border, text, accent, status, and
focus colors. Color is never the only status signal. A light/system theme may be
added without changing component contracts.

All workflows need visible focus, correct labels, predictable tab order, escape
behavior, focus restoration, reduced-motion support, and keyboard operation.
Dialogs trap focus only while modal. Toasts do not contain the only copy of an
error, and destructive actions expose target and consequence before confirmation.

## AI and developer experience

The dashboard should expose inspectable facts: field identifiers and types,
generated curl requests, event envelopes, actor/scopes, policy explanations,
migration previews, job attempts, and request IDs. These improve human debugging
and machine-assisted development without delegating authorization decisions to AI.
