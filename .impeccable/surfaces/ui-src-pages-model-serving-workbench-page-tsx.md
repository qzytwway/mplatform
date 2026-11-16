---
version: 1
slug: "ui-src-pages-model-serving-workbench-page-tsx"
primary_target: "ui/src/pages/model-serving/workbench-page.tsx"
related_targets: ["ui/src/pages/overview.tsx","ui/src/components/pod-overview.tsx"]
---

Scope: the console shell plus the three densest overview surfaces (model-serving Workbench, cluster Overview at /dashboard, resource detail summaries starting with Pod). Mode: Operate.

Audience and job: platform engineers and model engineers triaging incidents, running routine checks, and deploying changes on desktop. Constraints: logo, light and dark themes, en/zh i18n; standard shadcn controls and navigation stay.

## Direction contract

THESIS: Problems first, boxes last. The console reads like a status board: what is wrong sits at the top in its severity colour, everything else is ruled rows of measurements. It refuses the category default of equal-weight bordered cards with coloured icon tiles.

OWN-WORLD: Steel-ink world. Neutrals carry a small consistent blue-steel tint (hue ~250) in both themes; the sidebar is a committed deep steel rail in both themes, visibly its own surface. Hue elsewhere belongs to status only: red critical, amber warning, green healthy, blue info. Hairline rules instead of boxes; radius tightened to 6px; measurements set in the system mono with tabular figures; labels stay in the system sans. Focus ring and text selection take the steel accent.

STORY: Open any overview and know within a second whether anything is wrong, how bad, and where to click; then scan the figures; then act.

FIRST VIEWPORT: Workbench: title row with actions; a full-width severity band (fill = highest live severity: critical red tint, warning amber tint, calm when clear) listing items with their actions; below it one ruled strip of five mono figures with no cell boxes; then GPU nodes as ruled rows with capacity bars; then Inference Services. Header shows the current cluster beside the breadcrumb.

FORM: Pinned by the user ("控制室" / control room), translated to type, palette, density and one signature move: the severity band. seed:f318891e418988a5

FINISH: unreviewed and undocumented is unfinished; this build ends with the finish review, the verdict, DESIGN.md, and every shipping raster carrying its provenance
