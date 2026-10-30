# ClickFix brand

The product name is **ClickFix** (final, CF-15) and the domain is
**getclickfix.com** (see "Yayınlama" in `docs/DEVELOPMENT.md`). Technical names (`NAME_PREFIX`, database, bucket, Meilisearch
prefix) use lowercase `clickfix`.

Direction: **signal on ink**. Near-black ink surfaces, a warm paper white in
light mode, calm neutral greys for dense screens, and one electric green used
sparingly as the _signal_: focus, the primary action in dark mode, live states
and unread markers.

## Brand kit

The official logo kit was supplied by the owner (cursor mark in nine
artboards). The SVGs are cleaned (Illustrator ids and metadata removed,
geometry and colours untouched) and kept in
[`frontend/src/assets/brand/`](../frontend/src/assets/brand/). PNG, PDF and AI
sources are not in the repo. The geometry is also inlined in
[`components/brand/clickfix-paths.ts`](../frontend/src/components/brand/clickfix-paths.ts)
so no request is made for the logo in the UI. Keep both in sync.

There is no drawn wordmark in the kit. The name is **typeset**: lowercase
`clickfix` in Space Grotesk 600 with -0.04em tracking, next to the mark at
1.2x the cap size. Keep that recipe; do not retype it in another face.

The kit uses exactly three colours: signal green `#03FF13`, black `#000000` and
white `#FFFFFF`.

| File (`src/assets/brand/`)      | Artboard  | Art                        | Used for                                                                                      |
| ------------------------------- | --------- | -------------------------- | --------------------------------------------------------------------------------------------- |
| `clickfix-tile-green.svg`       | 2050 tile | Black cursor on green tile | **Hero / app icon**: `app/icon.svg`, `apple-icon.tsx`, `AppLogo variant="icon"`               |
| `clickfix-tile-ink.svg`         | 2050 tile | Green cursor on black tile | Dark-only placements of the app icon (store art, dark docs)                                   |
| `clickfix-tile-green-white.svg` | 2050 tile | White cursor on green tile | Reserved (single-colour print); not used in the app                                           |
| `clickfix-tile-ink-white.svg`   | 2050 tile | White cursor on black tile | Reserved (mono / grayscale contexts)                                                          |
| `clickfix-circle-ink.svg`       | 1832 disc | Green cursor on black disc | **In-product mark**: `AppMark` (sidebar, auth, landing nav, footer, wordmark lockup)          |
| `clickfix-circle-green.svg`     | 1832 disc | Black cursor on green disc | Avatars / badges on dark surfaces                                                             |
| `clickfix-circle-ink-white.svg` | 1832 disc | White cursor on black disc | Mono contexts                                                                                 |
| `clickfix-mark-green.svg`       | 1280x744  | Bare green cursor          | Green cursor on black or dark surfaces                                                        |
| `clickfix-mark-ink.svg`         | 1280x744  | Bare black cursor          | Black cursor on light surfaces; `AppMark glyph="cursor"` uses this geometry in `currentColor` |

Where the art appears in the app:

| Place                                           | File / component                                                                                                                                                                                                                                                                                                           |
| ----------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Favicon                                         | `frontend/src/app/icon.svg` = `clickfix-tile-green.svg`                                                                                                                                                                                                                                                                    |
| Apple touch / PWA icon                          | `frontend/src/app/apple-icon.tsx` draws the same green tile                                                                                                                                                                                                                                                                |
| Sidebar header, organization switcher           | `AppSidebarLogo` -> `AppMark` (ink disc, green cursor)                                                                                                                                                                                                                                                                     |
| Auth pages, landing nav, final CTA, legal pages | `AppWordmark` -> `AppMark` + typeset name                                                                                                                                                                                                                                                                                  |
| Footer                                          | `AppLogo variant="mark"`                                                                                                                                                                                                                                                                                                   |
| Empty states                                    | `AppMark glyph="cursor"` (bare cursor, `currentColor`)                                                                                                                                                                                                                                                                     |
| Web widget launcher                             | Not ClickFix branded on purpose: the widget runs on customers' sites and takes the organization's own accent colour and a chat glyph                                                                                                                                                                                       |
| Emails                                          | One branded layout for every email (CF-155): `backend/internal/platform/mail/layout.html.tmpl`, design option C (centered tile + typeset name, signal-dot eyebrow, tinted details box). `clickfix-tile-green.svg` rendered to an 80px PNG (`mail/assets/logo.png`), embedded by CID; text + HTML multipart, light and dark |
| Mobile                                          | `mobile/assets/images`: `icon.png` / `favicon.png` = `clickfix-tile-green.svg`; Android adaptive icon = black cursor on green (white for monochrome); splash = `clickfix-circle-ink.svg`; in-app `BrandMark` / `BrandTile` (`mobile/src/ui/brand.tsx`)                                                                     |

### Decisions (CF-15)

- **Hero variant**: the green tile (Asset 1) is the app icon. Black on signal
  green is the highest-contrast pairing in the kit and stays legible at 16 px
  (the cursor fills about 62% of the tile width).
- **Ink is the kit black `#000000`** wherever the brand colour is meant: the
  mark, the icon tile, text on signal green, the sidebar card, `--ink`,
  `--signal-foreground`. The dark-mode page `--background` stays a lifted
  `#0A0D0B` (and cards `#111412`), otherwise a black mark and the black sidebar
  card would melt into the page. Landing sections keep `#0A0D0B` as surface for
  the same reason (`brand.colors.surface`).
- **Green is exactly `#03FF13`** everywhere it is a fill. The darker
  `#087313` (`--signal-text`) exists only for green text and links on light
  backgrounds, because `#03FF13` on white is 1.4:1.
- **White** appears only as the cursor colour in the mono variants.

## Colours

| Token       | Hex                              | Where                                                                                           |
| ----------- | -------------------------------- | ----------------------------------------------------------------------------------------------- |
| Ink         | `#000000` (kit black)            | The mark's disc, the icon tile, the sidebar card, text on signal                                |
| Surface     | `#0A0D0B`                        | Dark `--background` and landing sections (lifted from ink so black stays visible)               |
| Ink raised  | `#111412` / `#141815`            | Dark `--card` / `--popover`                                                                     |
| Paper       | `#F7F6F2`                        | Light `--background`; cards stay white on it                                                    |
| Signal      | `#03FF13`                        | The cursor, dark `--primary`, `--ring` (dark), live / unread dots, landing CTAs. **Fill only.** |
| Signal text | `#087313` light / `#3DFF4B` dark | Green text and links (`text-signal-text`)                                                       |
| Signal soft | `#E6F7E7` light / `#12301A` dark | Tinted backgrounds behind signal content                                                        |
| Ink text    | `#0C110D`                        | Light `--foreground` and light `--primary`                                                      |
| Muted text  | `#5E655F` light / `#9AA39C` dark | `--muted-foreground`                                                                            |
| Border      | `#E2E1DA` light / white 9% dark  | `--border`                                                                                      |
| Focus ring  | `#0E9F22` light / `#03FF13` dark | `--ring`                                                                                        |
| Active nav  | `#16301A` bg, `#D9F5DB` text     | `--sidebar-accent` on the ink sidebar                                                           |
| Destructive | `#C4321F` light / `#FF6B57` dark | `--destructive`                                                                                 |

Primary action: in **dark** mode the default button is signal green with ink
text. In **light** mode the default button is ink with paper text, because a
neon fill on white has almost no edge contrast (1.4:1) and would shout on every
form; green stays reserved for focus, live state and the marketing CTAs
(`bg-signal text-signal-foreground`), where it is the only accent on screen.

CSS tokens are in [`frontend/src/styles/theme.css`](../frontend/src/styles/theme.css)
(Tailwind: `bg-ink`, `bg-paper`, `bg-signal`, `text-signal-foreground`,
`text-signal-text`, `bg-signal-soft`, `font-display`, `font-accent`); JS hex
aliases and the shader palettes in
[`frontend/src/config/brand.ts`](../frontend/src/config/brand.ts).

### Contrast (WCAG 2.x)

| Pair                                                    | Ratio             | Use                              |
| ------------------------------------------------------- | ----------------- | -------------------------------- |
| Ink `#000000` on signal `#03FF13`                       | 15.3:1            | Text on green buttons and badges |
| Signal `#03FF13` on ink `#000000`                       | 15.3:1            | Green text / icons on dark       |
| Signal `#03FF13` on white                               | **1.4:1**         | Never use green as text on light |
| Signal text `#087313` on paper `#F7F6F2`                | 5.6:1             | Links and green text, light      |
| Signal text `#087313` on signal soft `#E6F7E7`          | 5.5:1             | Tinted badge, light              |
| `#3DFF4B` on card `#111412`                             | 13.8:1            | Links and green text, dark       |
| Ink text `#0C110D` on paper                             | 17.6:1            | Body text, light                 |
| Paper on ink text (light primary button)                | 17.6:1            | Default button, light            |
| Muted `#5E655F` on paper / white / muted `#EFEEE9`      | 5.5 / 6.0 / 5.2:1 | Secondary text, light            |
| Muted `#9AA39C` on ink / card / muted `#1A1E1B`         | 7.6 / 7.2 / 6.5:1 | Secondary text, dark             |
| Foreground `#E8ECE9` on ink                             | 16.5:1            | Body text, dark                  |
| Focus ring `#0E9F22` on white                           | 3.5:1             | Non-text (≥ 3:1)                 |
| `#D9F5DB` on `#16301A`                                  | 12.3:1            | Active nav item                  |
| Sidebar text `#C9D1CB` on ink                           | 12.5:1            | Nav items                        |
| Destructive `#C4321F` on white / `#FF6B57` on `#111412` | 5.5 / 6.6:1       | Errors                           |

Ratios computed with the WCAG relative-luminance formula.

## Typography

| Role                                         | Face                     | Token                                      |
| -------------------------------------------- | ------------------------ | ------------------------------------------ |
| Display: headings, wordmark, big numbers     | Space Grotesk (variable) | `--font-display`, `.font-display`, `h1–h4` |
| UI text                                      | Geist                    | `--font-sans` (body default)               |
| Accent: eyebrows, step numbers, KPI numerals | Oxanium 500/600          | `--font-accent`, `.font-accent`            |
| Code, ids, keys                              | Geist Mono               | `--font-mono`                              |

Oxanium is never used for running text and is not preloaded.

## Shape and depth

- `--radius` 0.75rem; the sidebar card uses 0.625rem so icon-only items stay
  rounded squares.
- Crisp 1px borders carry structure; shadows are soft and low (`shadow-sm`),
  never coloured except the signal glow on live dots.
- Selection colour is signal at 35%.

## Usage rules

- Green is a fill. Text on green is ink. Green text on light uses
  `text-signal-text`.
- One signal per view: if a screen has a green primary CTA, other accents stay
  neutral.
- On dark surfaces the mark gets a hairline white ring (`tone="dark"` or
  `auto` in dark mode) so the ink disc does not disappear.
- Don't stretch, outline, recolour or rotate the mark; use only the kit pairings
  (green / black / white cursor on the matching tile, disc or bare); the black
  cursor on green tile is the app icon.

## In the panel

`@/components/brand`:

| Component                               | Renders                                       |
| --------------------------------------- | --------------------------------------------- |
| `AppLogo variant="logo"` / `"wordmark"` | Mark + typeset `clickfix`                     |
| `AppLogo variant="mark"` / `AppMark`    | Disc and cursor                               |
| `AppLogo variant="icon"`                | App icon tile (green square, black cursor)    |
| `AppWordmark`                           | Lockup; size with a `text-*` class            |
| `AppSidebarLogo`                        | Mark + name + subtitle for the sidebar header |

`tone` names the background: `light`, `dark`, or `auto` (default, follows the
colour mode).

## Panel look

- Sidebar: a floating ink card (`.sidebar-ink.dark` in `theme.css`) in both
  colour modes with a faint green glow; the active item gets a green tint and a
  glowing signal dot as a leading marker.
- `.signal-dot` (+ `data-pulse="true"`) is the shared live / presence / unread
  marker; the pulse stops under `prefers-reduced-motion`.
