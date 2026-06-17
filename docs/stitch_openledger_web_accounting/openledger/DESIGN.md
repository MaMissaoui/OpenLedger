---
name: OpenLedger
colors:
  surface: '#f7fafc'
  surface-dim: '#d7dadc'
  surface-bright: '#f7fafc'
  surface-container-lowest: '#ffffff'
  surface-container-low: '#f1f4f6'
  surface-container: '#ebeef0'
  surface-container-high: '#e5e9eb'
  surface-container-highest: '#e0e3e5'
  on-surface: '#181c1e'
  on-surface-variant: '#43474e'
  inverse-surface: '#2d3133'
  inverse-on-surface: '#eef1f3'
  outline: '#74777f'
  outline-variant: '#c4c6cf'
  surface-tint: '#455f88'
  primary: '#002045'
  on-primary: '#ffffff'
  primary-container: '#1a365d'
  on-primary-container: '#86a0cd'
  inverse-primary: '#adc7f7'
  secondary: '#555f71'
  on-secondary: '#ffffff'
  secondary-container: '#d6e0f6'
  on-secondary-container: '#596376'
  tertiary: '#002713'
  on-tertiary: '#ffffff'
  tertiary-container: '#003f23'
  on-tertiary-container: '#4bb278'
  error: '#ba1a1a'
  on-error: '#ffffff'
  error-container: '#ffdad6'
  on-error-container: '#93000a'
  primary-fixed: '#d6e3ff'
  primary-fixed-dim: '#adc7f7'
  on-primary-fixed: '#001b3c'
  on-primary-fixed-variant: '#2d476f'
  secondary-fixed: '#d9e3f9'
  secondary-fixed-dim: '#bdc7dc'
  on-secondary-fixed: '#121c2c'
  on-secondary-fixed-variant: '#3d4759'
  tertiary-fixed: '#91f8b8'
  tertiary-fixed-dim: '#74db9d'
  on-tertiary-fixed: '#002110'
  on-tertiary-fixed-variant: '#00522f'
  background: '#f7fafc'
  on-background: '#181c1e'
  surface-variant: '#e0e3e5'
typography:
  display-lg:
    fontFamily: Hanken Grotesk
    fontSize: 40px
    fontWeight: '700'
    lineHeight: 48px
  headline-md:
    fontFamily: Hanken Grotesk
    fontSize: 24px
    fontWeight: '600'
    lineHeight: 32px
  headline-sm:
    fontFamily: Hanken Grotesk
    fontSize: 20px
    fontWeight: '600'
    lineHeight: 28px
  body-lg:
    fontFamily: Inter
    fontSize: 16px
    fontWeight: '400'
    lineHeight: 24px
  body-md:
    fontFamily: Inter
    fontSize: 14px
    fontWeight: '400'
    lineHeight: 20px
  body-sm:
    fontFamily: Inter
    fontSize: 12px
    fontWeight: '400'
    lineHeight: 18px
  data-numeric:
    fontFamily: JetBrains Mono
    fontSize: 14px
    fontWeight: '500'
    lineHeight: 20px
    letterSpacing: -0.02em
  label-caps:
    fontFamily: Inter
    fontSize: 11px
    fontWeight: '700'
    lineHeight: 16px
    letterSpacing: 0.05em
rounded:
  sm: 0.125rem
  DEFAULT: 0.25rem
  md: 0.375rem
  lg: 0.5rem
  xl: 0.75rem
  full: 9999px
spacing:
  sidebar-width: 260px
  container-max-width: 1280px
  gutter: 1.5rem
  margin-page: 2rem
  stack-tight: 0.5rem
  stack-default: 1rem
  stack-loose: 2rem
---

## Brand & Style

The visual identity of the design system is anchored in **Corporate Modernism** with a focus on high-fidelity data density. It is designed to feel authoritative yet approachable, replacing the clutter often found in legacy accounting software with structured clarity. 

The personality is precise, transparent, and efficient. It targets professionals and small business owners who require a reliable environment for complex financial tasks. The UI utilizes a "functional-first" hierarchy, where whitespace is used strategically to separate logical ledger sections, ensuring that even data-heavy screens remain legible and stress-free.

## Colors

The palette is built on a foundation of trust and stability.
- **Primary (Deep Blue):** Used for navigation backgrounds, primary actions, and branding. It provides a "financial anchor" to the interface.
- **Secondary (Charcoal Gray):** Employed for text, icons, and structural borders to maintain high contrast without the harshness of pure black.
- **Success (Professional Green):** A balanced green specifically tuned for positive balances, profit indicators, and "Saved" states.
- **Neutral Surface:** A series of cool grays and off-whites that distinguish the sidebar from the main workspace, mimicking the clean pages of a physical ledger.

## Typography

Typography is the most critical tool for data clarity in this design system. 
- **Headlines:** Use **Hanken Grotesk** for a sharp, modern professional look that scales well in headers.
- **Body & UI:** **Inter** is used for its exceptional legibility at small sizes, particularly in forms and menus.
- **Numeric Data:** **JetBrains Mono** (or a clean monospaced alternative) is strictly reserved for financial figures, ensuring that decimals and digits align perfectly in tabular layouts for quick scanning.
- **Labeling:** Small-caps are used sparingly for table headers to differentiate them from the data rows.

## Layout & Spacing

This design system utilizes a **Fixed-Fluid Hybrid** model.
- **Sidebar:** A fixed 260px navigation column on the left containing the primary application hierarchy and organizational switcher.
- **Main Content:** A fluid area that expands to a maximum width of 1280px to prevent line lengths from becoming unreadable on ultra-wide monitors.
- **Grid:** A 12-column system is used within the content area for forms and dashboards.
- **Table Density:** Rows in data tables use a compact 40px height to maximize the information visible above the fold, while form inputs use a more comfortable 48px height to facilitate touch and click accuracy.

## Elevation & Depth

Hierarchy is established primarily through **Tonal Layering** and subtle borders rather than heavy shadows.
- **Surface 0 (Background):** The lightest neutral gray, used for the main application canvas.
- **Surface 1 (Cards/Tables):** Pure white surfaces that sit atop the background, defined by a 1px soft border (#E2E8F0).
- **Sidebar:** A slightly tinted or dark surface to visually separate "Control" from "Content."
- **Interactive States:** Subtle ambient shadows (4px blur, 5% opacity) appear only on hovered interactive elements (buttons, cards) to indicate "lift" without cluttering the flat aesthetic.

## Shapes

The design system employs **Soft** geometry. A standard 0.25rem (4px) corner radius is applied to buttons, input fields, and small UI components. This provides a professional, "tooled" feel that is less aggressive than sharp corners but more serious than highly rounded "pill" styles. Large containers like cards or the main content area use 0.5rem (8px) for a slightly softer framing of data.

## Components

- **Buttons:** Primary buttons use the Deep Blue background with white text. Secondary buttons use a Ghost style (border only) to maintain low visual noise.
- **Data Tables:** Headers are sticky with a subtle bottom border. Zebra striping is avoided in favor of a hover-state highlight on the entire row to assist in cross-horizontal reading.
- **Input Fields:** Use a 1px border that thickens and changes to Primary Blue on focus. Error states are clearly marked with a red border and a small trailing icon.
- **Chips/Badges:** Used for transaction statuses (e.g., "Cleared," "Pending," "Void"). These utilize low-saturation background tints with high-saturation text for readability.
- **Sidebar Nav:** Icons are line-based and paired with body-md weight text. The active state is indicated by a vertical 4px bar on the left edge and a slight background tint change.
- **Value Indicators:** Positive values in currency are colored Success Green; negative values are Error Red and wrapped in parentheses, following standard accounting practices.