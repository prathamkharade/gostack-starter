# UI / UX system

Principles: **clear navigation, one primary action per screen, instant feedback, works on a cheap phone,
accessible by default.** Everything common is pre-built; change it in one place.

## Where to change the look
`static/css/input.css`:
- `@theme` – brand colours (`--color-brand-*`), semantic tokens (`surface`, `subtle`, `fg`, `muted`, `border`,
  `danger`, `success`, `warning`), font stacks, `--radius-ui`.
- `[data-theme="dark"]` – dark values of the same tokens (components follow automatically).
- `@layer components` – `.btn*`, `.card`, `.input`, `.label`, `.field`, `.badge*`, `.alert*`, `.table`, `.dialog`, `.prose`, `.nav-link`.

## Components (`internal/views/components`)
| Component | Use |
|---|---|
| `Navbar`, `Footer`, `ThemeToggle`, `Breadcrumbs` | shell; edit `MainNav` to change menu items |
| `PageHeader(title, subtitle){actions}` | top of every page |
| `Card`, `Badge(text, tone)`, `Alert(kind, msg)`, `EmptyState(title, text){action}` | layout & feedback |
| `Button(variant, attrs)`, `LinkButton(href, variant)` | variants: primary, secondary, danger, ghost |
| `Field(FieldProps)`, `TextArea`, `Select(props, options)`, `Checkbox` | forms with label, hint, error, aria wiring |
| `ListControls(action, target, params){filters}`, `FilterSelect`, `FilterDate`, `SortSelect`, `Pagination` | list pages (see `search-filter-pagination.md`) |
| `Dialog(title)` | native `<dialog>` inside `x-data="dialog"` (open: `x-on:click="show"`) |
| `Toasts` | global; triggered by `httpx.Toast` |
| `Spinner` | shows during htmx requests |
| `Picture`, `Video`, `MediaPreview` | responsive optimised media |
| `Icon*` | inline SVG icons |

## Interaction patterns
- **Forms**: POST → 422 with inline errors, or redirect + flash. Never lose user input on error.
- **Lists**: search-as-you-type (300 ms debounce), filters & sort in the same form, URL always reflects state.
- **Destructive actions**: `hx-confirm` for small things; a `Dialog` with typed confirmation for irreversible ones.
- **Async work** (uploads): show a pending state that polls (`hx-trigger="every 2s"`) until ready.
- **Feedback**: toast for small confirmations, flash alert after navigation, inline message for field errors.
- **Empty states** always explain what to do next and offer the action.

## Accessibility checklist (WCAG 2.2 AA)
Labels for all inputs (`Field` does it), visible focus (global `:focus-visible`), skip link, landmarks
(`header/nav/main/footer`), `aria-current` in nav, `aria-live` on results/toasts, colour contrast via tokens,
touch targets ≥ 24 px (buttons are 36 px), reduced-motion respected, no information by colour alone,
16 px inputs (no iOS zoom). `make test-e2e` runs axe on public pages.

## Adding a page (checklist)
1. Handler sets `httpx.SetMeta(c, seo.Meta{Title, Description, Path, NoIndex})`.
2. Template wraps content in `@layouts.Base() { … }` and starts with `PageHeader`.
3. Only component classes / tokens; check mobile width (375 px) and dark mode.
