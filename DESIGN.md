---
name: PBManager
description: Managed PocketBase hosting with a clear, practical operating workspace.
colors:
  primary: "#254bdf"
  primary-deep: "#1737b1"
  ink: "#152b49"
  muted: "#586b85"
  cloud: "#f4f7fc"
  surface: "#ffffff"
  line: "#dce4ef"
  pale: "#edf2ff"
  field-line: "#bdcae0"
  success: "#176448"
  success-surface: "#e6f4ee"
  success-dot: "#228563"
  danger: "#a52c36"
  danger-deep: "#84212a"
  danger-surface: "#fbecee"
  warning: "#88510d"
  warning-surface: "#fff2d9"
  focus-peach: "#f6b69a"
typography:
  display:
    fontFamily: "Manrope, sans-serif"
    fontSize: "clamp(48px, 5.2vw, 76px)"
    fontWeight: 800
    lineHeight: 1.08
    letterSpacing: "-0.035em"
  headline:
    fontFamily: "Manrope, sans-serif"
    fontSize: "43px"
    fontWeight: 800
    lineHeight: 1.08
    letterSpacing: "-0.035em"
  title:
    fontFamily: "Manrope, sans-serif"
    fontSize: "34px"
    fontWeight: 800
    lineHeight: 1.2
    letterSpacing: "-0.035em"
  metric:
    fontFamily: "Manrope, sans-serif"
    fontSize: "31px"
    fontWeight: 800
    lineHeight: 1.25
    letterSpacing: "-0.035em"
  panel-title:
    fontFamily: "Manrope, sans-serif"
    fontSize: "1.125rem"
    fontWeight: 800
    lineHeight: 1.6
    letterSpacing: "-0.02em"
  body:
    fontFamily: "Manrope, sans-serif"
    fontSize: "1rem"
    lineHeight: 1.6
  body-public:
    fontFamily: "Manrope, sans-serif"
    fontSize: "16px"
    lineHeight: 1.65
  action:
    fontFamily: "Manrope, sans-serif"
    fontSize: ".9375rem"
    fontWeight: 600
    lineHeight: 1.6
  action-public:
    fontFamily: "Manrope, sans-serif"
    fontSize: "1rem"
    fontWeight: 700
    lineHeight: 1.4
  label:
    fontFamily: "Manrope, sans-serif"
    fontSize: ".875rem"
    fontWeight: 600
    lineHeight: 1.6
rounded:
  small: "5px"
  field: "7px"
  control: "8px"
  icon: "10px"
  note: "12px"
  panel: "14px"
  prominent: "16px"
  band: "20px"
  hero: "24px"
spacing:
  micro: "8px"
  control: "12px"
  compact: "16px"
  panel: "22px"
  room: "24px"
  section: "36px"
  wide: "48px"
components:
  button-primary:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.surface}"
    typography: "{typography.action}"
    rounded: "{rounded.control}"
    padding: "11px 16px"
  button-primary-hover:
    backgroundColor: "{colors.primary-deep}"
  button-secondary:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink}"
    typography: "{typography.action}"
    rounded: "{rounded.control}"
    padding: "11px 16px"
  button-secondary-hover:
    backgroundColor: "{colors.pale}"
  button-danger:
    backgroundColor: "{colors.danger}"
    textColor: "{colors.surface}"
    typography: "{typography.action}"
    rounded: "{rounded.control}"
    padding: "11px 16px"
  button-danger-hover:
    backgroundColor: "{colors.danger-deep}"
  button-public-primary:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.surface}"
    typography: "{typography.action-public}"
    rounded: "{rounded.control}"
    padding: "12px 21px"
  button-public-on-blue:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.primary}"
    typography: "{typography.action-public}"
    rounded: "{rounded.control}"
    padding: "12px 21px"
  navigation-active:
    backgroundColor: "{colors.primary}"
    textColor: "{colors.surface}"
    rounded: "{rounded.control}"
    padding: "12px 14px"
  field:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink}"
    typography: "{typography.body}"
    rounded: "{rounded.field}"
    padding: "8px 10px"
  status-success:
    backgroundColor: "{colors.success-surface}"
    textColor: "{colors.success}"
    typography: "{typography.label}"
    rounded: "{rounded.small}"
    padding: "4px 7px"
  status-danger:
    backgroundColor: "{colors.danger-surface}"
    textColor: "{colors.danger}"
    typography: "{typography.label}"
    rounded: "{rounded.small}"
    padding: "4px 7px"
  status-warning:
    backgroundColor: "{colors.warning-surface}"
    textColor: "{colors.warning}"
    typography: "{typography.label}"
    rounded: "{rounded.small}"
    padding: "4px 7px"
  panel:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink}"
    rounded: "{rounded.panel}"
  instance-receipt:
    backgroundColor: "{colors.surface}"
    textColor: "{colors.ink}"
    rounded: "{rounded.prominent}"
    padding: "26px"
---

# Design System: PBManager

## Overview

**Creative North Star: "The Public Utility Service Board"**

PBManager gives a developer's PocketBase a dependable home. Its visual language combines confident cobalt information fields, cloud-white working surfaces, ink-blue lettering, square-ended rules, and softly rectangular controls. The tone is welcoming and practical; concise labels and visible service states do the explaining.

Public surfaces can use broad cobalt fields and large lettering. The administrator workspace uses the same identity with white panels on a pale cloud canvas, compact information, and restrained saturation. Operational meaning comes first: status colors accompany readable labels, measurements use aligned numerals, and examples remain visibly distinct from live infrastructure.

**Key Characteristics:**
- Cobalt identity with quieter operational surfaces.
- Locally served Manrope for both expressive headlines and precise controls.
- Flat panels, fine rules, and selective overlays.
- Semantic status colors paired with explicit text.
- An interactive instance receipt as a product-specific demonstration pattern.

This document records the implemented source in `deployment/github-pages/landing.html`, `frontend/src/styles/global.css`, and `frontend/src/pages/index.astro`, following the chosen direction in `.impeccable/surfaces/docs-index-html.md`. This document captures the source system; browser finish review is recorded separately. It does not assert a passed visual review.

## Colors

The palette places a clear service blue beside cool, readable neutrals; green, red, and amber carry operational meaning. The frontmatter is the normative palette. The sidecar's synthesized eight-step tonal ramps are reference strips, not additional shipped UI colors.

### Primary

- **Service Cobalt** (`primary`): brand marks, primary actions, active workspace navigation, progress, and public information fields. The landing page exposes it as `--blue`; the workspace uses `--primary`.
- **Deep Service Cobalt** (`primary-deep`): hover state for cobalt actions.
- **Pale Cobalt** (`pale`): supporting notes, small icon grounds, counts, and secondary-action hover surfaces.

### Neutral

- **Ink Blue** (`ink`): headings, important values, ordinary workspace text, and the standard toast background.
- **Slate Blue** (`muted`): secondary copy, table metadata, helper text, and inactive navigation.
- **Cloud White** (`cloud`): the workspace canvas and public dashboard framing.
- **Working White** (`surface`): panels, fields, navigation, receipts, and reversed buttons.
- **Blue-Gray Rule** (`line`): panel outlines, table structure, and section divisions.
- **Field Rule** (`field-line`): the stronger edge around editable fields.

### Named Rules

**The State Has Meaning Rule.** Use success green for healthy or completed states, danger red for errors and destructive actions, and warning amber for attention or work in progress. Pair every state with words; color and dots alone are insufficient.

**The Quiet Workspace Rule.** Keep the administrator canvas cloud-white and its working panels white. Concentrate cobalt in actions, active destinations, and selected data; broad cobalt fields belong to expressive public surfaces.

The success, danger, and warning foreground/surface pairs are semantic rather than additional brand accents. The success-dot token supplies the compact health marker. Focus Peach is the public page's visible keyboard-focus outline; the workspace uses cobalt focus outlines.

## Typography

**Display Font:** Manrope, with sans-serif fallback.  
**Body Font:** Manrope, with sans-serif fallback.  
**Label/Mono Font:** Manrope for labels; `ui-monospace, SFMono-Regular, monospace` for technical fragments where the source calls for them.

**Character:** Wide, open letterforms make the hosting offer approachable while heavy, tightly tracked headings establish a clear service identity. The same family keeps operating controls familiar and legible.

Three local font files supply regular, semibold, and extrabold weights (400, 600, 800). Font synthesis is disabled. Some incumbent declarations request intermediate weights, including public buttons (700); these select from the supplied faces rather than establishing more font assets.

### Hierarchy

- **Display:** the public hosting statement; fluid on desktop, with responsive overrides and a larger wide-screen setting (82px from 1600px).
- **Headline:** public section statements; the base showcase/FAQ role uses the headline token, while other sections vary with their composition.
- **Title:** the administrator page title; reduces to mobile size (28px at 760px and below).
- **Panel title:** administrator section headings use the panel-title role (18px at the default root size).
- **Metric:** prominent resource measurements; use tabular numerals. Desktop size reduces with available width.
- **Body:** workspace body, form labels, and editable inputs use the body role (16px at the default root size). Public prose ranges from 16px to 18px, with the more open public-body rhythm. Public paragraphs are capped at a readable measure (65ch), with narrower supporting copy where the layout needs it.
- **Action:** workspace controls and navigation use the action size (15px at the default root size); public navigation and primary actions use the larger public role (16px). The compact public variants remain at metadata size.
- **Label:** metadata, helper copy, table headings, and status pills use the label size (14px at the default root size). The increased roles use rem units and stay readable on narrow screens; body copy and form inputs retain their larger size.

**The Clear Label Rule.** Keep labels concise and use sentence case. Use the established rem-based roles for readable controls and metadata; preserve body and input sizes on narrow screens rather than shrinking the type to fit.

## Layout

Public content centers in a bounded container (1200px maximum, initially 80px total horizontal inset). Broad colored sections sit slightly outside that grid (16px side margins). Public vertical spacing is generous, with section gaps commonly spanning roughly 70–124px; the core hero is a two-column layout that becomes one column at 800px. Its editable receipt then centers at up to 500px width. At 1100px, 800px, and 420px, type, gutters, navigation, and section spacing tighten. At 1600px the hero gets more breathing room. Mobile navigation opens as a white bordered menu.

The administrator shell uses a fixed left navigation (260px), a white top bar (70px), and a bounded main area (1580px maximum) with desktop padding (33px 36px 24px). Four metrics share one white rail, divided by rules. The next information row uses a wider storage panel beside a narrower server panel, followed by an instances table and activity feed. Panel content commonly uses horizontal padding (22px) and gaps around 22–24px.

At 1250px the workspace navigation narrows (238px) and main gutters tighten (25px). At 1050px the overview becomes one column; the incumbent server summary is hidden. At 760px navigation becomes an off-canvas drawer (260px), the top bar uses an automatic height with a minimum (64px) and vertical padding (12px), main gutters reduce (17px), and metrics form a two-by-two rail. Heading groups, storage headings, legends, metric descriptions, and panel footers wrap to preserve the larger type. The table keeps its columns inside a horizontal scroll container; preserve that container's relative positioning so visually hidden labels remain contained. Small domain/resource forms also stack at 600px.

## Elevation & Depth

The system is mostly flat. White surfaces, cool canvas tones, and fine borders establish working regions. Ordinary buttons and panels have no resting shadow. Depth is reserved for temporary overlays and the public demonstration receipt.

### Shadow Vocabulary

- **Public receipt** (`0 24px 60px #102f9026`): the editable example on the cobalt field; its slight desktop rotation (1deg) disappears when stacked.
- **Workspace menu** (`0 12px 35px #152b4920`): temporary row action menus.
- **Workspace dialog** (`0 25px 80px #152b4926`): focused tasks above the workspace; the backdrop uses a restrained blur (3px).
- **Workspace toast** (`0 12px 32px #152b4920`): transient feedback.
- **Public mobile menu** (`0 12px 24px #152b4912`): navigation when expanded over the page.

**The Working Surface Rule.** Use flat, bordered working panels. Reserve substantial shadows for overlays and the public receipt rather than applying elevation to every container.

## Shapes

Controls are softly rectangular, with the control radius shared by buttons and navigation. Editable fields use the slightly tighter field radius; small status labels use the small radius. Main workspace panels use the panel radius, while prominent receipts and dialogs use the prominent radius. Public colored bands are broader and more rounded. The brand mark is a vertical rounded rectangle containing a simple outlined server icon.

Circular geometry has a functional role: health dots, step markers, and profile avatars. Rules and segmented storage rails stay crisp and direct. Avoid introducing a second decorative geometry into operational screens.

## Components

### Buttons

Confident, concise, and easy to identify. Workspace primary buttons use cobalt with white text, the control radius, semibold action text, roomier padding (11px 16px), and a minimum height (44px desktop; general buttons may reduce to 40px on mobile, while the page-heading action keeps 44px). Content can grow the control beyond its minimum. Primary hover uses deep cobalt. Secondary buttons use white, ink text, and a fine rule; hover uses pale cobalt. Destructive actions use the danger pair and deepen on hover. Buttons have no elevation.

Public actions are larger (48px minimum height, 12px 21px padding). A white button reverses the action on a cobalt field. The compact header action is smaller. Keyboard focus is explicit: workspace buttons/links use an outline (3px, 4px offset), public controls use the peach outline (3px, 5px offset). Field controls use their tighter outline (2px, 2px offset).

### Chips

Small, softly rectangular labels use compact padding (4px 7px), semibold text, and no border in the final semantic variants. Use green for healthy/running, gray for neutral/stopped, red for failed, and amber for pending/provisioning states. Include the state label next to any status dot.

### Cards / Containers

White panels have a fine blue-gray border and the panel radius. Metrics are grouped within one panel-shaped rail rather than displayed as separate floating cards. Interior dividers create the desktop columns and mobile two-by-two layout. Panel headers, body regions, and footers have deliberate padding; do not assume every panel has one universal padding value.

### Inputs / Fields

White fields use ink text at the body size, a blue-gray edge, and the field radius. Search, resource, and select controls have a minimum height (42px). Resource fields use compact padding (8px 10px); named-instance inputs have roomier padding (11px 12px). Name/domain composites keep an attached suffix within the same outline. Focus on a composite is shown around its wrapper. Search retains an explicit accessible label; placeholders are supporting hints. Errors use danger text beside the relevant field.

### Navigation

The workspace sidebar uses a white ground and muted labels. The current destination is a cobalt rounded rectangle with white heavy lettering; inactive rows receive a pale hover surface. Small counts align at the trailing edge. The mobile drawer keeps the same navigation structure and an explicit open/close control. Public navigation is lighter, with compact semibold links and a separate hosting action.

### Instance Receipt

The public example is a white receipt with a server heading, editable project name, illustrative domain, full-width action, three named stages, and a final status line. It shows idle, working, complete, and error states in words. Working markers spin; completed stages turn green. The example is explicitly illustrative and never implies that it provisioned real infrastructure.

### Feedback and Motion

Toasts use ink with white text; errors use danger red. Action menus and dialogs follow the overlay shadow vocabulary. Hover transitions are short (150ms in the workspace; 180ms on the public page), drawer and FAQ transitions are restrained (200ms), and loading spinners communicate ongoing work. Toast entrance is a brief upward fade (200ms). Honor reduced motion: disable animation and transitions, and remove smooth scrolling when requested by the system.

## Do's and Don'ts

### Do:

- **Do** use the cobalt, cloud-white, and ink-blue palette across public and administrator surfaces.
- **Do** pair semantic status color with a readable state label.
- **Do** use the local Manrope assets and preserve the compact, sentence-case control language.
- **Do** keep tables horizontally scrollable inside their own contained region on narrow screens.
- **Do** keep keyboard focus visible and honor reduced-motion preferences.
- **Do** distinguish illustrative deployment progress and demo measurements from live operations.

### Don't:

- **Don't** make every operational panel a raised card or fill the workspace with large cobalt backgrounds.
- **Don't** use semantic red, green, or amber as arbitrary decorative accents.
- **Don't** replace service labels with unexplained colored dots or icons.
- **Don't** turn public demo feedback into claims of real provisioning, availability, or performance.
- **Don't** shrink supporting labels below the established functional text minima.
