---
name: MPlatform
description: A control-room console for Kubernetes clusters and the models served on their GPUs.
colors:
  steel-paper: "oklch(0.985 0.003 262)"
  steel-ink: "oklch(0.22 0.02 262)"
  surface-white: "oklch(1 0 0)"
  steel-primary: "oklch(0.3 0.05 262)"
  steel-primary-foreground: "oklch(0.98 0.005 262)"
  steel-secondary: "oklch(0.95 0.008 262)"
  steel-muted: "oklch(0.96 0.007 262)"
  steel-muted-ink: "oklch(0.5 0.03 262)"
  steel-accent: "oklch(0.94 0.012 262)"
  hairline: "oklch(0.905 0.012 262)"
  input-stroke: "oklch(0.88 0.014 262)"
  steel-ring: "oklch(0.55 0.15 262)"
  capacity-steel: "oklch(0.45 0.08 262)"
  steel-rail: "oklch(0.26 0.045 262)"
  steel-rail-ink: "oklch(0.88 0.02 262)"
  steel-rail-active: "oklch(0.58 0.17 262)"
  steel-rail-hover: "oklch(0.32 0.05 262)"
  rail-active-ink: "oklch(0.99 0 0)"
  status-critical: "oklch(0.56 0.2 27)"
  status-warning: "oklch(0.62 0.14 78)"
  status-healthy: "oklch(0.55 0.14 150)"
  status-info: "oklch(0.55 0.17 255)"
typography:
  page-title:
    fontFamily: "ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, 'Noto Sans', sans-serif"
    fontSize: "1.5rem"
    fontWeight: 700
    lineHeight: 1.33
  figure-lg:
    fontFamily: "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace"
    fontSize: "1.875rem"
    fontWeight: 600
    lineHeight: 1.2
    fontFeature: "tnum"
  figure-md:
    fontFamily: "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace"
    fontSize: "1.5rem"
    fontWeight: 600
    lineHeight: 1.33
    fontFeature: "tnum"
  figure-sm:
    fontFamily: "ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace"
    fontSize: "1.125rem"
    fontWeight: 600
    lineHeight: 1.55
    fontFeature: "tnum"
  section-title:
    fontFamily: "ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, 'Noto Sans', sans-serif"
    fontSize: "0.875rem"
    fontWeight: 600
    lineHeight: 1.43
  body:
    fontFamily: "ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, 'Noto Sans', sans-serif"
    fontSize: "0.875rem"
    fontWeight: 400
    lineHeight: 1.43
  label:
    fontFamily: "ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, 'Helvetica Neue', Arial, 'Noto Sans', sans-serif"
    fontSize: "0.75rem"
    fontWeight: 400
    lineHeight: 1.33
rounded:
  sm: "2px"
  md: "4px"
  lg: "6px"
  xl: "10px"
  full: "9999px"
spacing:
  section-gap: "24px"
  strip-cell: "16px 20px"
  summary-cell: "12px 16px"
  band-inset: "12px 16px"
  section-head: "8px 0"
components:
  button-primary:
    backgroundColor: "{colors.steel-primary}"
    textColor: "{colors.steel-primary-foreground}"
    rounded: "{rounded.md}"
    padding: "8px 16px"
    height: "36px"
  button-outline:
    backgroundColor: "{colors.steel-paper}"
    textColor: "{colors.steel-ink}"
    rounded: "{rounded.md}"
    padding: "8px 16px"
    height: "36px"
  button-outline-hover:
    backgroundColor: "{colors.steel-accent}"
  button-ghost-hover:
    backgroundColor: "{colors.steel-accent}"
    textColor: "{colors.steel-ink}"
    rounded: "{rounded.md}"
  stat-strip-cell:
    textColor: "{colors.steel-ink}"
    typography: "{typography.figure-md}"
    padding: "{spacing.strip-cell}"
  summary-cell:
    textColor: "{colors.steel-ink}"
    typography: "{typography.figure-sm}"
    padding: "{spacing.summary-cell}"
  severity-band-calm:
    textColor: "{colors.steel-ink}"
    rounded: "{rounded.md}"
  severity-band-critical:
    textColor: "{colors.steel-ink}"
    rounded: "{rounded.md}"
  severity-band-warning:
    textColor: "{colors.steel-ink}"
    rounded: "{rounded.md}"
  sidebar-rail:
    backgroundColor: "{colors.steel-rail}"
    textColor: "{colors.steel-rail-ink}"
  sidebar-item-active:
    backgroundColor: "{colors.steel-rail-active}"
    textColor: "{colors.rail-active-ink}"
    rounded: "{rounded.md}"
    height: "28px"
  sidebar-item-hover:
    backgroundColor: "{colors.steel-rail-hover}"
  alert-count:
    backgroundColor: "{colors.status-critical}"
    textColor: "{colors.steel-paper}"
    rounded: "{rounded.full}"
    padding: "0 6px"
  capacity-bar:
    backgroundColor: "{colors.capacity-steel}"
    rounded: "{rounded.full}"
    height: "6px"
---

# Design System: MPlatform

## Overview

**Creative North Star: "The Control Room"**

MPlatform reads like a status board, not a gallery of tiles. What is wrong sits at the top of an overview in its severity colour; everything else is ruled rows of measurements. The world is steel ink: every neutral, in both themes, carries the same small blue-steel tint (hue 262, taken from the logo blue), and the navigation is a committed deep steel rail that is visibly its own surface in light and dark alike. Hue anywhere else belongs to status and nothing else.

Density is operator density. Overview figures sit in strips divided by hairline rules instead of being boxed one per card; measurements are set in the system monospace with tabular figures so columns of numbers align and scan; labels stay in the system sans at small sizes in the muted steel ink. Corners are tight (6px base). Depth is almost absent: the world separates content with 1px rules and tonal tints, not shadows.

The world is new and was applied to the shell and the three densest overview surfaces (model-serving Workbench, cluster overview, workload and Pod detail summaries). Many secondary panels (charts, events, information blocks, forms, list pages) still use the inherited shadcn Card container; that is legacy, not the pattern for new surfaces.

**Key Characteristics:**
- Steel-tinted neutrals (hue 262) in both themes; no zero-chroma grey.
- A deep steel sidebar rail in both themes, with a brighter steel fill marking the active item.
- Status hues (red, amber, green, blue) carry meaning only.
- Summary figures in hairline-ruled strips, set mono and tabular.
- A severity band at the top of an overview whose fill is the worst live severity.
- Tight 6px base radius; flat surfaces; steel focus ring, caret and selection.

## Colors

A steel-ink palette: one cool blue-steel hue across all neutrals, with four status hues held in reserve for meaning. Values below are the light theme; each token has a dark-theme counterpart in `ui/src/styles/theme.css` (`.dark`) and in the sidecar.

### Primary
- **Steel Ink Primary** (steel-primary): the default button fill and primary action; in dark mode it inverts to a pale steel so the primary button stays the highest-contrast control.
- **Steel Ring** (steel-ring): the focus ring, the text caret, and text selection (mixed to 30% over the page). It is the only saturated non-status steel outside the rail.

### Secondary
- **Steel Rail** (steel-rail): the sidebar surface in both themes; with the inset shell it also frames the main panel. Rail text uses **Rail Ink** (steel-rail-ink), secondary rail text at 70% of it, hover rows use **Rail Hover** (steel-rail-hover), and the active item takes **Rail Active** (steel-rail-active) with near-white **Rail Active Ink** (rail-active-ink). In the dark theme the rail is a slightly darker shade of the content's own tint (same hue, near-equal chroma), never bluer or lighter than the content, so the two read as one surface family.
- **Capacity Steel** (capacity-steel): the fill of capacity bars (GPU allocation). It is deliberately not a status hue: allocation is a quantity, not a health signal.

### Tertiary
- **Critical Red** (status-critical): critical severity, failing state, not-ready counts, the severity band's critical tint, the solid sidebar alert count, destructive actions.
- **Warning Amber** (status-warning): warning severity and the band's warning tint.
- **Healthy Green** (status-healthy): ready/all-clear check marks and ready dots.
- **Info Blue** (status-info): informational states only.

### Neutral
- **Steel Paper** (steel-paper): the page background.
- **Surface White** (surface-white): card and popover surfaces where containers remain.
- **Steel Ink** (steel-ink): body text and figures.
- **Muted Steel Ink** (steel-muted-ink): labels, hints, secondary detail lines, inactive icons.
- **Steel Secondary / Muted / Accent** (steel-secondary, steel-muted, steel-accent): secondary buttons, skeletons and track fills, hover backgrounds.
- **Hairline** (hairline): every 1px rule and border; in dark mode it becomes a 13%-alpha steel line.
- **Input Stroke** (input-stroke): form-control borders.

### Named Rules
**The Status Hue Rule.** Red, amber, green and blue mean critical, warning, healthy and info, and appear only when they carry that meaning. The cluster overview dropped its colour-coded icon tiles and the GPU capacity bar moved off the info blue to Capacity Steel for this reason: an inventory count or an allocation is not a status.

**The One Tint Rule.** Every neutral carries the same hue (262) at low chroma, in both themes. Don't introduce a zero-chroma grey or a second neutral hue.

## Typography

**Body Font:** the system UI sans stack (ui-sans-serif, system-ui, then platform fallbacks)
**Label/Mono Font:** the system monospace stack (ui-monospace, SFMono-Regular, Menlo, then fallbacks)

**Character:** Product-native type. The sans carries words; the mono carries numbers. The whole ramp is in rem against a root size bound to the user's display-scale setting, so it scales as one.

### Hierarchy
- **Page title** (700, 1.5rem): one per page, in the title row beside the page actions.
- **Figure, large** (mono 600, 1.875rem, tabular): headline counts in the cluster overview strip.
- **Figure, medium** (mono 600, 1.5rem, tabular): the Workbench stat strip.
- **Figure, small** (mono 600, 1.125rem, tabular): values in resource-detail summary strips.
- **Section title** (600, 0.875rem): section headers on overview surfaces and the severity band header.
- **Body** (400, 0.875rem): rows, detail lines, empty states.
- **Label** (400, 0.75rem, muted steel ink): cell labels in summary strips, hints beside section titles, severity group headings (in their status colour).

### Named Rules
**The Figure Rule.** A value that is a number, or a string made only of digits, spaces, slashes, dots and commas (a count or a ratio such as "3 / 4"), is set in the mono with tabular figures. Words stay in the sans even when they sit in a figure slot (a status like "Running").

## Layout

Desktop-only console. A fixed sidebar rail on the left, a header row (3.5rem) carrying the sidebar toggle, the **current cluster name** with a server icon, then the breadcrumb, search and actions on the right. Content sits in the inset main panel.

Overview pages stack full-width bands vertically with a 24px gap: title row with actions, severity band, figure strip, then sections. Sections have no box: a section header is a single row (title, optional muted hint, optional right-aligned link) over a 1px rule, with content 12px below. Responsive behavior uses container queries on the main panel and on each overview component, not viewport breakpoints: strips go from two columns to four, five or six as the container widens.

Spacing rhythm observed: strip cells 16px by 20px; summary cells 12px by 16px; band header and rows 12px by 16px; section headers 8px vertical.

## Elevation & Depth

Flat by construction. The world separates content with 1px hairline rules and with tonal tints (the severity band's 10% status fill, the rail's deep steel), never with shadows. The few shadows on screen are inherited from shadcn primitives (a hairline extra-small shadow on buttons, a small shadow on the inset main panel) and are not part of the world's vocabulary.

### Named Rules
**The Rule-Not-Box Rule.** On overview surfaces, group by hairline rules and alignment, not by wrapping each item in a bordered, shadowed card.

## Shapes

Tight, functional corners from one base radius (6px): 2px, 4px, 6px and 10px steps. Controls and the severity band use 4px; the base 6px is the reference for panels; 10px is the inherited Card corner and is not used by new surfaces. Fully rounded shapes are reserved for small indicators: status dots, capacity bars and the alert count pill. Strips and sections have no corners at all because they have no box; they are bounded by top and bottom rules.

## Components

### Buttons
Standard shadcn buttons recoloured by the steel tokens.
- **Shape:** gently squared (4px), 36px tall by default, 32px small.
- **Primary:** Steel Ink Primary fill with near-white text, 16px horizontal padding.
- **Outline:** page-coloured fill with a hairline border; hover takes the steel accent.
- **Ghost:** no fill until hover (steel accent).
- **Destructive:** Critical Red with white text, used for delete actions.
- **Focus:** a 3px steel ring at 50% around the control.

### Ruled Figure Strip (signature)
The default for summary measurements (Workbench stats, cluster overview counts, workload and Pod summaries). One grid of cells bounded by a rule above and below, with a vertical hairline between cells and no per-cell border, background or radius. Each cell: muted label, mono tabular figure, optional detail line. The detail line turns Critical Red when it reports a problem (for example "2 not ready"); otherwise it is muted, with a green check for "all ready". Cells that have a page are links and take a faint muted hover fill.

### Severity Band (signature)
The first thing under the title row on the Workbench. A full-width bordered block (4px corners) whose border and fill come from the worst live item: critical gives a 10% red tint with a 40% red border (20% / 60% in dark), warning gives the same in amber (15% / 55% in dark), and clear gives a hairline border only with a green "all good" line. The header row (section title and count badge) takes the band's border colour for its divider. Items are grouped under small status-coloured headings ("Critical 1") and each item is a link row with a status glyph and its action.

### Sections
A header row over a hairline rule, then content. No container. Collapsible groups use a ruled row (top and bottom hairlines) with a rotating chevron as the trigger.

### Capacity Rows
GPU nodes as list rows: a 6px status dot, the node name, a muted type hint, and the allocation as a mono "allocated / capacity" figure on the right, over a 6px rounded capacity bar (Capacity Steel on a muted track).

### Navigation
- **Rail:** the deep steel sidebar in both themes. Group headings and collapsible group triggers use rail ink at 70%; open groups go full rail ink and medium weight.
- **Items:** 28px rows; hover takes Rail Hover; the active item is a solid Rail Active fill with near-white Rail Active Ink.
- **Alert count:** a solid Critical Red pill with page-coloured text on the active-alerts item, counting only alerts that report a problem.
- **Header:** current cluster shown before the breadcrumb, separated by vertical hairlines.

### Cards / Containers (legacy)
The shadcn Card (10px corners, hairline border, Surface White fill, 24px vertical padding) still wraps charts, event lists, information blocks, forms and detail panels across roughly sixty files. It renders in the steel palette but predates the world. New overview surfaces use strips and sections instead; Card remains acceptable for genuinely self-contained interactive panels.

## Do's and Don'ts

### Do:
- **Do** put the severity band first on an overview and colour it by the worst live item.
- **Do** lay summary measurements out as a ruled figure strip: rules above and below, hairlines between cells, no per-cell box.
- **Do** set every number or count/ratio in the system mono with tabular figures (the Figure Rule).
- **Do** keep neutrals on the single steel tint (hue 262) and use the steel ring for focus, caret and selection.
- **Do** show the current cluster in the header wherever it changes what the user sees.
- **Do** keep the sidebar a deep steel rail in both themes, with the active item as a solid Rail Active fill.
- **Don't** give the dark-theme rail more chroma or lightness than the content area; a blue rail beside a near-black content area reads as two products.

### Don't:
- **Don't** use red, amber, green or blue for anything that is not a status (no colour-coded icon tiles, no info-blue capacity bars).
- **Don't** wrap each figure of a summary in its own bordered card.
- **Don't** add decorative shadows; separate with hairlines and tints.
- **Don't** introduce zero-chroma greys or a second neutral hue.
- **Don't** treat the 10px Card corner as the house radius for new surfaces.
