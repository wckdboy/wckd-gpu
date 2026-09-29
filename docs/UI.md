# wckd-gpu UI

Ops console for timed GPU sessions. Dense, dark, one mint Start. Tokens live in `apps/web/src/styles/tokens.css` and are applied by `apps/web/src/styles.css`. The PWA and the Tauri shell share this UI.

## Color

| Token | Hex | Use |
| --- | --- | --- |
| `--bg` | `#0B0C0E` | Page background |
| `--elevated` | `#12141A` | Top bar, nav, sticky dock |
| `--panel` | `#181B22` | Cards, tables, dialogs |
| `--line` | `#2A2F3A` | Hairline borders only |
| `--text` | `#E8EAED` | Primary text |
| `--text-secondary` | `#9AA3B2` | Supporting copy |
| `--text-muted` | `#6B7380` | Meta, empty titles' surroundings |
| `--accent` | `#3DDC97` | Start, live phase, recommended mark |
| `--warn` | `#F5A524` | Cost risk, T−15, sample banners |
| `--danger` | `#FF5C5C` | Stop, deadline elapsed, errors |
| `--info` | `#5B8CFF` | Secure tier |

No pure white surfaces. No rainbow charts. Mint is the Start button, the ready pip, and the recommended mark — not a general highlight.

## Type

UI stack: Inter, then `ui-sans-serif`. Sizes in use are 12 / 13 / 14 / 16. Body is 13px.

Mono stack: JetBrains Mono, then `ui-monospace`. Use it for rates, timers, session ids, offer SKUs, and empty-state titles. Numerals are tabular.

Uppercase tracking is only for `.tag` labels (SESSION, OFFERS, BEFORE START). Nav, titles, and body stay sentence case.

## Motion

Panels, banners, and dialogs enter in 160ms ease-out (`--ease`). No bounce, no spring.

The Start button scales to 0.98 on press. It is disabled while there is no locked live offer, while a start is provisioning, and in the PWA (`Desktop required for Start.`).

The countdown and the cost ticker use tabular numerals. Inside the last 15 minutes the countdown flashes amber. The ticker updates every 250ms from catalog `$/hr × elapsed` since `created_at`. It is a running catalog figure, not an invoice. `cost_actual_usd` stays on the fact row when the CLI has it.

## Screens

1. **Home.** Session card or `no session`. One sticky Start dock: est. total USD, $/hr, duration, and the line “Hard stop at deadline; unpaid orphans are a failure.” Secondary actions are ghost buttons.
2. **Offers.** Filters, then a compact table: score, vendor, SKU, VRAM, tier, $/hr, est. total. Rank 1 is marked REC. Click a row to lock it. Secure tier uses the info blue. Skeleton rows while ranking. Same Start dock. Dry run is ghost.
3. **Session live.** Phase chip (`pending`, `hydrating`, `ready`, `draining`, plus `failed` / `terminated`), countdown, catalog ticker, Open UI, Extend, Stop. The ticker is catalog `$/hr × elapsed`, not an invoice. Open UI follows the session proxy URL and allows `http://` as well as `https://`, which vendor proxies use. Extend explains that `wckd session extend` is not in P1. Stop confirms: “Stop runs drain → S3 sync → terminate. Force skip drain only if you accept data loss risk.” Force still requires the typed word `DESTROY`.
4. **Projects, presets, settings.** Same surfaces. They do not get a mint primary. Config check, create, and preset pick are outline buttons.

## Empty, loading, error

Empty states are a mono title plus one recovery action: Rank offers on desktop, Preview sample rank / Preview live layout in the PWA.

Sample rows are labeled SAMPLE. They are not quotes and cannot Start, dry-run, or stop a pod.

Errors are inline banners. When a ranked list is on screen, the banner offers the next row as failover. No toasts.

## Bridge

GPU actions (config, offers, start, status, stop) go through the Tauri CLI bridge. The PWA is an installable shell. It stores project ids and non-secret paths only. Secrets stay in the CLI `.env` or the process environment.
