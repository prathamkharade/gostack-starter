# Working with Claude Code efficiently (fewest tokens, best results)

## How this repo saves tokens
- `CLAUDE.md` is short (loaded every session): map + rules. Deep knowledge lives in `docs/` and in
  **skills** (`.claude/skills/*`), which load only when used.
- One example module (`internal/items`) shows every pattern → Claude copies instead of exploring.
- `make check` is the single "is it done?" command; output is short.
- Generated/vendor files are denied in `.claude/settings.json`, so they never fill the context.
- Common safe commands are pre-approved (no permission round-trips).

## Habits (from Anthropic's cost guidance)
1. **One task per conversation.** Run `/clear` when switching to unrelated work (`/rename` first if you want
   to `/resume` later). Stale context is paid for on every message.
2. **Plan big things first**: press Shift+Tab to enter plan mode, review the plan, then let it implement.
3. **Be specific**: "add a due date filter to projects list" beats "improve projects".
   Mention files: `@internal/projects/handlers.go`.
4. **Use Sonnet for most work**, Opus for architecture or tricky bugs (`/model`). Lower `/effort` for simple edits.
5. **Stop early**: press Esc when it heads the wrong way; `/rewind` to undo.
6. Check usage with `/usage`; `/context` shows what fills the context.
7. Let it verify itself: "…then run make check and fix failures".

## Slash commands in this repo
| Command | Use |
|---|---|
| `/new-app <idea>` | turn the starter into your product (spec → data map → resources → tests) |
| `/add-resource projects: name:text!, status:enum(active,done), due:date` | one CRUD resource end-to-end |
| `/compliance-review` | privacy/security/a11y review of current changes |
| `/init` | (built-in) not needed – CLAUDE.md already exists |

## Prompt: start a new application from an idea
Paste this in a fresh Claude Code session (VS Code panel), replacing the text in brackets:

```
/new-app [Describe the app in 3-8 sentences: who uses it, what they do, what makes it valuable.
Example: "A booking app for independent yoga teachers. Teachers create classes with time, place,
capacity and price. Students browse public class pages, book a spot and get an email confirmation.
Teachers see attendee lists and can cancel classes, which notifies attendees. Payments come later.
Users are mostly in Germany and the UK."]
Constraints: keep the existing stack and rules in CLAUDE.md, EU-grade privacy, mobile-first UI,
minimal new dependencies. Work phase by phase and run make check after each phase.
```

## Prompt templates for everyday work
- **Feature**: "In plan mode: add <feature>. Touch only <area>. Follow the items pattern. Add an integration
  test. Run make check."
- **Bug**: "Bug: <what happens> vs <expected>. Steps: <…>. Find the cause, add a failing test, fix it, run make check."
- **UI change**: "Change <page/component> to <…>. Use existing components and tokens from docs/ui.md; no inline
  styles/scripts. Show me the templ diff."
- **Review**: "/compliance-review" then "Fix the issues marked trivial."
- **Docs sync**: "Update CLAUDE.md map and the relevant docs/ file for the changes in this branch. Keep CLAUDE.md under 120 lines."
